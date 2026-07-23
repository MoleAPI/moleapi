package controller

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/constant"
	"github.com/QuantumNous/new-api/model"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// GetAllModelsMeta 获取模型列表（分页）
func GetAllModelsMeta(c *gin.Context) {

	pageInfo := common.GetPageQuery(c)
	status := c.Query("status")
	syncOfficial := c.Query("sync_official")
	modelsMeta, total, err := model.SearchModels("", "", status, syncOfficial, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 批量填充附加字段，提升列表接口性能
	enrichModels(modelsMeta)

	// 统计供应商计数（全部数据，不受分页影响）
	vendorCounts, _ := model.GetVendorModelCounts()

	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(modelsMeta)
	common.ApiSuccess(c, gin.H{
		"items":         modelsMeta,
		"total":         total,
		"page":          pageInfo.GetPage(),
		"page_size":     pageInfo.GetPageSize(),
		"vendor_counts": vendorCounts,
	})
}

// SearchModelsMeta 搜索模型列表
func SearchModelsMeta(c *gin.Context) {

	keyword := c.Query("keyword")
	vendor := c.Query("vendor")
	status := c.Query("status")
	syncOfficial := c.Query("sync_official")
	pageInfo := common.GetPageQuery(c)

	modelsMeta, total, err := model.SearchModels(keyword, vendor, status, syncOfficial, pageInfo.GetStartIdx(), pageInfo.GetPageSize())
	if err != nil {
		common.ApiError(c, err)
		return
	}
	// 批量填充附加字段，提升列表接口性能
	enrichModels(modelsMeta)
	vendorCounts, _ := model.GetVendorModelCounts()
	pageInfo.SetTotal(int(total))
	pageInfo.SetItems(modelsMeta)
	common.ApiSuccess(c, gin.H{
		"items":         modelsMeta,
		"total":         total,
		"page":          pageInfo.GetPage(),
		"page_size":     pageInfo.GetPageSize(),
		"vendor_counts": vendorCounts,
	})
}

// GetModelMeta 根据 ID 获取单条模型信息
func GetModelMeta(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	var m model.Model
	if err := model.DB.First(&m, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	enrichModels([]*model.Model{&m})
	common.ApiSuccess(c, &m)
}

// CreateModelMeta 新建模型
func CreateModelMeta(c *gin.Context) {
	var m model.Model
	if err := c.ShouldBindJSON(&m); err != nil {
		common.ApiError(c, err)
		return
	}
	if m.ModelName == "" {
		common.ApiErrorMsg(c, "模型名称不能为空")
		return
	}
	// 名称冲突检查
	if dup, err := model.IsModelNameDuplicated(0, m.ModelName); err != nil {
		common.ApiError(c, err)
		return
	} else if dup {
		common.ApiErrorMsg(c, "模型名称已存在")
		return
	}

	if err := m.Insert(); err != nil {
		common.ApiError(c, err)
		return
	}
	model.RefreshPricing()
	common.ApiSuccess(c, &m)
}

// UpdateModelMeta 更新模型
func UpdateModelMeta(c *gin.Context) {
	statusOnly := c.Query("status_only") == "true"

	var m model.Model
	if err := c.ShouldBindJSON(&m); err != nil {
		common.ApiError(c, err)
		return
	}
	if m.Id == 0 {
		common.ApiErrorMsg(c, "缺少模型 ID")
		return
	}

	if statusOnly {
		// 只更新状态，防止误清空其他字段
		if err := model.DB.Model(&model.Model{}).Where("id = ?", m.Id).Update("status", m.Status).Error; err != nil {
			common.ApiError(c, err)
			return
		}
	} else {
		// 名称冲突检查
		if dup, err := model.IsModelNameDuplicated(m.Id, m.ModelName); err != nil {
			common.ApiError(c, err)
			return
		} else if dup {
			common.ApiErrorMsg(c, "模型名称已存在")
			return
		}

		if err := m.Update(); err != nil {
			common.ApiError(c, err)
			return
		}
	}
	model.RefreshPricing()
	common.ApiSuccess(c, &m)
}

// DeleteModelMeta 删除模型
func DeleteModelMeta(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if err := model.DB.Delete(&model.Model{}, id).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	model.RefreshPricing()
	common.ApiSuccess(c, nil)
}

type modelDescriptionExportItem struct {
	ModelName       string            `json:"model_name"`
	Description     string            `json:"description"`
	DescriptionI18N map[string]string `json:"description_i18n,omitempty"`
	Icon            string            `json:"icon,omitempty"`
	Tags            string            `json:"tags,omitempty"`
	VendorName      string            `json:"vendor_name,omitempty"`
	Endpoints       string            `json:"endpoints,omitempty"`
	Status          int               `json:"status"`
	SyncOfficial    int               `json:"sync_official"`
	NameRule        int               `json:"name_rule"`
}

type modelDescriptionVendorExport struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Icon        string `json:"icon,omitempty"`
	Status      int    `json:"status"`
}

type modelDescriptionExport struct {
	Version    int                            `json:"version"`
	ExportedAt int64                          `json:"exported_at"`
	Vendors    []modelDescriptionVendorExport `json:"vendors,omitempty"`
	Models     []modelDescriptionExportItem   `json:"models"`
}

type modelDescriptionImportItem struct {
	ModelName       string            `json:"model_name"`
	Description     *string           `json:"description,omitempty"`
	DescriptionI18N map[string]string `json:"description_i18n,omitempty"`
	Icon            *string           `json:"icon,omitempty"`
	Tags            *string           `json:"tags,omitempty"`
	VendorName      *string           `json:"vendor_name,omitempty"`
	Endpoints       *string           `json:"endpoints,omitempty"`
	Status          *int              `json:"status,omitempty"`
	SyncOfficial    *int              `json:"sync_official,omitempty"`
	NameRule        *int              `json:"name_rule,omitempty"`
}

type modelDescriptionImport struct {
	Vendors []modelDescriptionVendorExport `json:"vendors,omitempty"`
	Models  []modelDescriptionImportItem   `json:"models"`
}

func cleanModelDescriptionTranslations(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for locale, description := range in {
		locale = strings.TrimSpace(locale)
		description = strings.TrimSpace(description)
		if locale == "" || description == "" {
			continue
		}
		out[locale] = description
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func modelDescriptionTranslations(raw model.JSONValue) map[string]string {
	if len(raw) == 0 {
		return nil
	}
	var translations map[string]string
	if err := common.Unmarshal([]byte(raw), &translations); err != nil {
		return nil
	}
	return cleanModelDescriptionTranslations(translations)
}

func marshalModelDescriptionTranslations(translations map[string]string) (model.JSONValue, error) {
	cleaned := cleanModelDescriptionTranslations(translations)
	if len(cleaned) == 0 {
		return nil, nil
	}
	b, err := common.Marshal(cleaned)
	if err != nil {
		return nil, err
	}
	return model.JSONValue(b), nil
}

func ExportModelDescriptions(c *gin.Context) {
	var modelsMeta []model.Model
	if err := model.DB.Order("model_name ASC").Find(&modelsMeta).Error; err != nil {
		common.ApiError(c, err)
		return
	}

	var vendors []model.Vendor
	if err := model.DB.Order("name ASC").Find(&vendors).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	vendorByID := make(map[int]model.Vendor, len(vendors))
	vendorItems := make([]modelDescriptionVendorExport, 0, len(vendors))
	for _, v := range vendors {
		vendorByID[v.Id] = v
		vendorItems = append(vendorItems, modelDescriptionVendorExport{
			Name:        v.Name,
			Description: v.Description,
			Icon:        v.Icon,
			Status:      v.Status,
		})
	}

	items := make([]modelDescriptionExportItem, 0, len(modelsMeta))
	for _, m := range modelsMeta {
		vendorName := ""
		if vendor, ok := vendorByID[m.VendorID]; ok {
			vendorName = vendor.Name
		}
		items = append(items, modelDescriptionExportItem{
			ModelName:       m.ModelName,
			Description:     m.Description,
			DescriptionI18N: modelDescriptionTranslations(m.DescriptionI18N),
			Icon:            m.Icon,
			Tags:            m.Tags,
			VendorName:      vendorName,
			Endpoints:       m.Endpoints,
			Status:          m.Status,
			SyncOfficial:    m.SyncOfficial,
			NameRule:        m.NameRule,
		})
	}

	body, err := common.Marshal(modelDescriptionExport{
		Version:    2,
		ExportedAt: common.GetTimestamp(),
		Vendors:    vendorItems,
		Models:     items,
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}

	c.Header("Content-Disposition", `attachment; filename="model-descriptions.json"`)
	c.Data(http.StatusOK, "application/json; charset=utf-8", body)
}

func ImportModelDescriptions(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)
	var req modelDescriptionImport
	if err := common.DecodeJson(c.Request.Body, &req); err != nil {
		common.ApiErrorMsg(c, "invalid model description backup")
		return
	}
	if len(req.Models) == 0 {
		common.ApiErrorMsg(c, "no model descriptions to import")
		return
	}

	now := common.GetTimestamp()
	var vendors []model.Vendor
	if err := model.DB.Find(&vendors).Error; err != nil {
		common.ApiError(c, err)
		return
	}
	vendorByName := make(map[string]*model.Vendor, len(vendors))
	for i := range vendors {
		vendorByName[vendors[i].Name] = &vendors[i]
	}
	vendorDetailsByName := make(map[string]modelDescriptionVendorExport, len(req.Vendors))
	for _, vendor := range req.Vendors {
		name := strings.TrimSpace(vendor.Name)
		if name != "" {
			vendorDetailsByName[name] = vendor
		}
	}

	updated := 0
	created := 0
	createdVendors := 0
	processedVendors := make(map[string]struct{})
	skipped := make([]string, 0)
	err := model.DB.Transaction(func(tx *gorm.DB) error {
		for _, item := range req.Models {
			name := strings.TrimSpace(item.ModelName)
			if name == "" {
				continue
			}

			vendorID := 0
			if item.VendorName != nil {
				vendorName := strings.TrimSpace(*item.VendorName)
				if vendorName != "" {
					if _, done := processedVendors[vendorName]; !done {
						detail, hasDetail := vendorDetailsByName[vendorName]
						if hasDetail && detail.Status != 0 && detail.Status != 1 {
							return fmt.Errorf("invalid vendor status for %s", vendorName)
						}
						if vendor, ok := vendorByName[vendorName]; ok {
							if hasDetail {
								if err := tx.Model(vendor).Updates(map[string]interface{}{
									"description":  strings.TrimSpace(detail.Description),
									"icon":         strings.TrimSpace(detail.Icon),
									"status":       detail.Status,
									"updated_time": now,
								}).Error; err != nil {
									return err
								}
							}
						} else {
							status := 1
							if hasDetail {
								status = detail.Status
							}
							vendor := model.Vendor{
								Name:        vendorName,
								Description: strings.TrimSpace(detail.Description),
								Icon:        strings.TrimSpace(detail.Icon),
								Status:      status,
								CreatedTime: now,
								UpdatedTime: now,
							}
							if err := tx.Create(&vendor).Error; err != nil {
								return err
							}
							vendorByName[vendorName] = &vendor
							createdVendors++
						}
						processedVendors[vendorName] = struct{}{}
					}
					if vendor, ok := vendorByName[vendorName]; ok {
						vendorID = vendor.Id
					}
				}
			}

			var existing model.Model
			if err := tx.Where("model_name = ?", name).First(&existing).Error; err != nil {
				if !errors.Is(err, gorm.ErrRecordNotFound) {
					return err
				}
				newModel := model.Model{
					ModelName:    name,
					Status:       1,
					SyncOfficial: 1,
					CreatedTime:  now,
					UpdatedTime:  now,
				}
				if item.Description != nil {
					newModel.Description = strings.TrimSpace(*item.Description)
				}
				if item.DescriptionI18N != nil {
					raw, err := marshalModelDescriptionTranslations(item.DescriptionI18N)
					if err != nil {
						return err
					}
					newModel.DescriptionI18N = raw
				}
				if item.Icon != nil {
					newModel.Icon = strings.TrimSpace(*item.Icon)
				}
				if item.Tags != nil {
					newModel.Tags = strings.TrimSpace(*item.Tags)
				}
				if item.Endpoints != nil {
					newModel.Endpoints = strings.TrimSpace(*item.Endpoints)
				}
				if item.Status != nil {
					if *item.Status != 0 && *item.Status != 1 {
						return fmt.Errorf("invalid status for model %s", name)
					}
					newModel.Status = *item.Status
				}
				if item.SyncOfficial != nil {
					if *item.SyncOfficial != 0 && *item.SyncOfficial != 1 {
						return fmt.Errorf("invalid sync_official for model %s", name)
					}
					newModel.SyncOfficial = *item.SyncOfficial
				}
				if item.NameRule != nil {
					if *item.NameRule < model.NameRuleExact || *item.NameRule > model.NameRuleSuffix {
						return fmt.Errorf("invalid name_rule for model %s", name)
					}
					newModel.NameRule = *item.NameRule
				}
				if item.VendorName != nil {
					newModel.VendorID = vendorID
				}
				if err := tx.Create(&newModel).Error; err != nil {
					return err
				}
				created++
				continue
			}

			updates := map[string]interface{}{
				"updated_time": now,
			}
			if item.Description != nil {
				updates["description"] = strings.TrimSpace(*item.Description)
			}
			if item.DescriptionI18N != nil {
				translations := modelDescriptionTranslations(existing.DescriptionI18N)
				if translations == nil {
					translations = map[string]string{}
				}
				for locale, description := range cleanModelDescriptionTranslations(item.DescriptionI18N) {
					translations[locale] = description
				}
				raw, err := marshalModelDescriptionTranslations(translations)
				if err != nil {
					return err
				}
				updates["description_i18n"] = raw
			}
			if item.Icon != nil {
				updates["icon"] = strings.TrimSpace(*item.Icon)
			}
			if item.Tags != nil {
				updates["tags"] = strings.TrimSpace(*item.Tags)
			}
			if item.Endpoints != nil {
				updates["endpoints"] = strings.TrimSpace(*item.Endpoints)
			}
			if item.Status != nil {
				if *item.Status != 0 && *item.Status != 1 {
					return fmt.Errorf("invalid status for model %s", name)
				}
				updates["status"] = *item.Status
			}
			if item.SyncOfficial != nil {
				if *item.SyncOfficial != 0 && *item.SyncOfficial != 1 {
					return fmt.Errorf("invalid sync_official for model %s", name)
				}
				updates["sync_official"] = *item.SyncOfficial
			}
			if item.NameRule != nil {
				if *item.NameRule < model.NameRuleExact || *item.NameRule > model.NameRuleSuffix {
					return fmt.Errorf("invalid name_rule for model %s", name)
				}
				updates["name_rule"] = *item.NameRule
			}
			if item.VendorName != nil {
				updates["vendor_id"] = vendorID
			}
			if len(updates) == 1 {
				continue
			}
			if err := tx.Model(&existing).Updates(updates).Error; err != nil {
				return err
			}
			updated++
		}
		return nil
	})
	if err != nil {
		common.ApiError(c, err)
		return
	}
	if updated > 0 {
		model.RefreshPricing()
	}

	common.ApiSuccess(c, gin.H{
		"updated_models":  updated,
		"created_models":  created,
		"created_vendors": createdVendors,
		"skipped_models":  skipped,
	})
}

// enrichModels 批量填充附加信息：端点、渠道、分组、计费类型，避免 N+1 查询
func enrichModels(models []*model.Model) {
	if len(models) == 0 {
		return
	}

	// 1) 拆分精确与规则匹配
	exactNames := make([]string, 0)
	exactIdx := make(map[string][]int) // modelName -> indices in models
	ruleIndices := make([]int, 0)
	for i, m := range models {
		if m == nil {
			continue
		}
		if m.NameRule == model.NameRuleExact {
			exactNames = append(exactNames, m.ModelName)
			exactIdx[m.ModelName] = append(exactIdx[m.ModelName], i)
		} else {
			ruleIndices = append(ruleIndices, i)
		}
	}

	// 2) 批量查询精确模型的绑定渠道
	channelsByModel, _ := model.GetBoundChannelsByModelsMap(exactNames)

	// 3) 精确模型：端点从缓存、渠道批量映射、分组/计费类型从缓存
	for name, indices := range exactIdx {
		chs := channelsByModel[name]
		for _, idx := range indices {
			mm := models[idx]
			if mm.Endpoints == "" {
				eps := model.GetModelSupportEndpointTypes(mm.ModelName)
				if b, err := common.Marshal(eps); err == nil {
					mm.Endpoints = string(b)
				}
			}
			mm.BoundChannels = chs
			mm.EnableGroups = model.GetModelEnableGroups(mm.ModelName)
			mm.QuotaTypes = model.GetModelQuotaTypes(mm.ModelName)
		}
	}

	if len(ruleIndices) == 0 {
		return
	}

	// 4) 一次性读取定价缓存，内存匹配所有规则模型
	pricings := model.GetPricing()

	// 为全部规则模型收集匹配名集合、端点并集、分组并集、配额集合
	matchedNamesByIdx := make(map[int][]string)
	endpointSetByIdx := make(map[int]map[constant.EndpointType]struct{})
	groupSetByIdx := make(map[int]map[string]struct{})
	quotaSetByIdx := make(map[int]map[int]struct{})

	for _, p := range pricings {
		for _, idx := range ruleIndices {
			mm := models[idx]
			var matched bool
			switch mm.NameRule {
			case model.NameRulePrefix:
				matched = strings.HasPrefix(p.ModelName, mm.ModelName)
			case model.NameRuleSuffix:
				matched = strings.HasSuffix(p.ModelName, mm.ModelName)
			case model.NameRuleContains:
				matched = strings.Contains(p.ModelName, mm.ModelName)
			}
			if !matched {
				continue
			}
			matchedNamesByIdx[idx] = append(matchedNamesByIdx[idx], p.ModelName)

			es := endpointSetByIdx[idx]
			if es == nil {
				es = make(map[constant.EndpointType]struct{})
				endpointSetByIdx[idx] = es
			}
			for _, et := range p.SupportedEndpointTypes {
				es[et] = struct{}{}
			}

			gs := groupSetByIdx[idx]
			if gs == nil {
				gs = make(map[string]struct{})
				groupSetByIdx[idx] = gs
			}
			for _, g := range p.EnableGroup {
				gs[g] = struct{}{}
			}

			qs := quotaSetByIdx[idx]
			if qs == nil {
				qs = make(map[int]struct{})
				quotaSetByIdx[idx] = qs
			}
			qs[p.QuotaType] = struct{}{}
		}
	}

	// 5) 汇总所有匹配到的模型名称，批量查询一次渠道
	allMatchedSet := make(map[string]struct{})
	for _, names := range matchedNamesByIdx {
		for _, n := range names {
			allMatchedSet[n] = struct{}{}
		}
	}
	allMatched := make([]string, 0, len(allMatchedSet))
	for n := range allMatchedSet {
		allMatched = append(allMatched, n)
	}
	matchedChannelsByModel, _ := model.GetBoundChannelsByModelsMap(allMatched)

	// 6) 回填每个规则模型的并集信息
	for _, idx := range ruleIndices {
		mm := models[idx]

		// 端点并集 -> 序列化
		if es, ok := endpointSetByIdx[idx]; ok && mm.Endpoints == "" {
			eps := make([]constant.EndpointType, 0, len(es))
			for et := range es {
				eps = append(eps, et)
			}
			if b, err := common.Marshal(eps); err == nil {
				mm.Endpoints = string(b)
			}
		}

		// 分组并集
		if gs, ok := groupSetByIdx[idx]; ok {
			groups := make([]string, 0, len(gs))
			for g := range gs {
				groups = append(groups, g)
			}
			mm.EnableGroups = groups
		}

		// 配额类型集合（保持去重并排序）
		if qs, ok := quotaSetByIdx[idx]; ok {
			arr := make([]int, 0, len(qs))
			for k := range qs {
				arr = append(arr, k)
			}
			sort.Ints(arr)
			mm.QuotaTypes = arr
		}

		// 渠道并集
		names := matchedNamesByIdx[idx]
		channelSet := make(map[string]model.BoundChannel)
		for _, n := range names {
			for _, ch := range matchedChannelsByModel[n] {
				key := ch.Name + "_" + strconv.Itoa(ch.Type)
				channelSet[key] = ch
			}
		}
		if len(channelSet) > 0 {
			chs := make([]model.BoundChannel, 0, len(channelSet))
			for _, ch := range channelSet {
				chs = append(chs, ch)
			}
			mm.BoundChannels = chs
		}

		// 匹配信息
		mm.MatchedModels = names
		mm.MatchedCount = len(names)
	}
}

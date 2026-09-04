package controller

import (
	"net/http"
	"strconv"

	"github.com/QuantumNous/new-api/model"
	"github.com/QuantumNous/new-api/pkg/channelprobe"
	perfmetrics "github.com/QuantumNous/new-api/pkg/perf_metrics"
	"github.com/QuantumNous/new-api/setting/operation_setting"
	"github.com/QuantumNous/new-api/setting/ratio_setting"

	"github.com/gin-gonic/gin"
	"github.com/samber/lo"
)

func GetPerfMetricsSummary(c *gin.Context) {
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	activeGroups := append(lo.Keys(ratio_setting.GetGroupRatioCopy()), "auto")
	result, err := perfmetrics.QuerySummaryAll(hours, activeGroups)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func GetPerfMetrics(c *gin.Context) {
	modelName := c.Query("model")
	if modelName == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "model is required",
		})
		return
	}

	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	result, err := perfmetrics.Query(perfmetrics.QueryParams{
		Model: modelName,
		Group: c.Query("group"),
		Hours: hours,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	result.Groups = filterActiveGroups(result.Groups)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    result,
	})
}

func GetChannelSuccessMetrics(c *gin.Context) {
	hours := 24
	if rawHours := c.Query("hours"); rawHours != "" {
		if parsed, err := strconv.Atoi(rawHours); err == nil {
			hours = parsed
		}
	}

	result, err := perfmetrics.QueryChannelSuccess(hours)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	channels, err := model.GetAllChannelsWithoutKey()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"channels":       result.Channels,
			"probe_overview": buildChannelProbeOverview(channels),
		},
	})
}

type channelProbeOverviewItem struct {
	ChannelId   int    `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Model       string `json:"model"`
	Level       string `json:"level,omitempty"`
	Status      string `json:"status"`
	RecentPass  int    `json:"recent_pass"`
	RecentTotal int    `json:"recent_total"`
	LastTestAt  int64  `json:"last_test_at,omitempty"`
}

type channelProbeOverview struct {
	Enabled         bool                       `json:"enabled"`
	Mode            string                     `json:"mode"`
	EnabledChannels int                        `json:"enabled_channels"`
	TotalModels     int                        `json:"total_models"`
	Healthy         int                        `json:"healthy"`
	Degraded        int                        `json:"degraded"`
	Pending         int                        `json:"pending"`
	Items           []channelProbeOverviewItem `json:"items"`
}

func buildChannelProbeOverview(channels []*model.Channel) channelProbeOverview {
	monitorSetting := operation_setting.GetMonitorSetting()
	overview := channelProbeOverview{
		Enabled: monitorSetting.AutoTestChannelEnabled,
		Mode:    monitorSetting.ChannelTestType,
		Items:   make([]channelProbeOverviewItem, 0),
	}
	for _, channel := range selectChannelsForAutomaticTest(channels, monitorSetting.ChannelTestMode) {
		if channel == nil {
			continue
		}
		enabled := channel.GetOtherSettings().ChannelProbeEnabled
		if enabled != nil && !*enabled {
			continue
		}
		overview.EnabledChannels++
		state := channelprobe.StateFromOtherInfo(channel.OtherInfo)
		for _, modelName := range channelTestModels(channel) {
			modelState, ok := state.Models[modelName]
			status := modelState.Status
			if !ok || status == "" {
				status = channelprobe.StatusPending
			}
			item := channelProbeOverviewItem{
				ChannelId:   channel.Id,
				ChannelName: channel.Name,
				Model:       modelName,
				Level:       modelState.StableLevel,
				Status:      status,
				RecentTotal: len(modelState.Recent),
				LastTestAt:  modelState.LastTestAt,
			}
			for _, sample := range modelState.Recent {
				if sample.Outcome == channelprobe.OutcomePass {
					item.RecentPass++
				}
			}
			switch status {
			case channelprobe.StatusHealthy:
				overview.Healthy++
			case channelprobe.StatusDegraded:
				overview.Degraded++
			default:
				overview.Pending++
			}
			overview.TotalModels++
			overview.Items = append(overview.Items, item)
		}
	}
	return overview
}

func filterActiveGroups(groups []perfmetrics.GroupResult) []perfmetrics.GroupResult {
	activeRatios := ratio_setting.GetGroupRatioCopy()
	return lo.Filter(groups, func(g perfmetrics.GroupResult, _ int) bool {
		_, ok := activeRatios[g.Group]
		return ok || g.Group == "auto"
	})
}

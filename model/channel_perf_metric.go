package model

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ChannelPerfMetric stores hourly upstream-attempt totals for channel stability reporting.
type ChannelPerfMetric struct {
	Id           int   `json:"id" gorm:"primaryKey"`
	ChannelId    int   `json:"channel_id" gorm:"uniqueIndex:idx_channel_perf_channel_bucket,priority:1"`
	BucketTs     int64 `json:"bucket_ts" gorm:"uniqueIndex:idx_channel_perf_channel_bucket,priority:2;index:idx_channel_perf_bucket_ts"`
	RequestCount int64 `json:"request_count" gorm:"default:0"`
	SuccessCount int64 `json:"success_count" gorm:"default:0"`
}

func (ChannelPerfMetric) TableName() string {
	return "channel_perf_metrics"
}

func UpsertChannelPerfMetric(metric *ChannelPerfMetric) error {
	if metric == nil || metric.RequestCount == 0 {
		return nil
	}
	return DB.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "channel_id"},
			{Name: "bucket_ts"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"request_count": gorm.Expr("channel_perf_metrics.request_count + ?", metric.RequestCount),
			"success_count": gorm.Expr("channel_perf_metrics.success_count + ?", metric.SuccessCount),
		}),
	}).Create(metric).Error
}

func GetChannelPerfMetrics(startTs int64, endTs int64) ([]ChannelPerfMetric, error) {
	var metrics []ChannelPerfMetric
	err := DB.Model(&ChannelPerfMetric{}).
		Where("bucket_ts >= ? AND bucket_ts <= ?", startTs, endTs).
		Order("bucket_ts ASC").
		Find(&metrics).Error
	return metrics, err
}

func GetChannelNamesByIDs(ids []int) (map[int]string, error) {
	names := make(map[int]string, len(ids))
	if len(ids) == 0 {
		return names, nil
	}
	var channels []struct {
		Id   int
		Name string
	}
	if err := DB.Model(&Channel{}).Select("id, name").Where("id IN ?", ids).Find(&channels).Error; err != nil {
		return nil, err
	}
	for _, channel := range channels {
		names[channel.Id] = channel.Name
	}
	return names, nil
}

func DeleteChannelPerfMetricsBefore(cutoffTs int64) error {
	if cutoffTs <= 0 {
		return nil
	}
	return DB.Where("bucket_ts < ?", cutoffTs).Delete(&ChannelPerfMetric{}).Error
}

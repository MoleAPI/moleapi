package operation_setting

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/setting/config"
)

type MonitorSetting struct {
	AutoTestChannelEnabled  bool    `json:"auto_test_channel_enabled"`
	AutoTestChannelMinutes  float64 `json:"auto_test_channel_minutes"`
	ChannelTestMode         string  `json:"channel_test_mode"`
	ChannelTestConcurrency  int     `json:"channel_test_concurrency"`
	ModelHealthCheckMinutes int     `json:"model_health_check_minutes"`
}

const (
	ChannelTestModeScheduledAll = "scheduled_all"
	ChannelTestModeAutoDetect   = "auto_detect"
	ChannelTestModeAutoDisable  = "auto_disable"

	// Legacy values remain accepted so existing saved settings keep their meaning.
	ChannelTestModeAutoBanOnly     = "auto_ban_only"
	ChannelTestModePassiveRecovery = "passive_recovery"
	ChannelTestModeScheduledProbes = "scheduled_probes"

	ChannelTestConcurrencyOptionKey  = "monitor_setting.channel_test_concurrency"
	DefaultChannelTestConcurrency    = 1
	MaxChannelTestConcurrency        = 32
	ModelHealthCheckMinutesOptionKey = "monitor_setting.model_health_check_minutes"
	DefaultModelHealthCheckMinutes   = 1
	MaxModelHealthCheckMinutes       = 1440
)

// 默认配置
var monitorSetting = MonitorSetting{
	AutoTestChannelEnabled:  false,
	AutoTestChannelMinutes:  10,
	ChannelTestMode:         ChannelTestModeScheduledAll,
	ChannelTestConcurrency:  DefaultChannelTestConcurrency,
	ModelHealthCheckMinutes: DefaultModelHealthCheckMinutes,
}

func init() {
	// 注册到全局配置管理器
	config.GlobalConfig.Register("monitor_setting", &monitorSetting)
}

func GetMonitorSetting() *MonitorSetting {
	if os.Getenv("CHANNEL_TEST_FREQUENCY") != "" {
		frequency, err := strconv.Atoi(os.Getenv("CHANNEL_TEST_FREQUENCY"))
		if err == nil && frequency > 0 {
			monitorSetting.AutoTestChannelEnabled = true
			monitorSetting.AutoTestChannelMinutes = float64(frequency)
			monitorSetting.ChannelTestMode = ChannelTestModeScheduledAll
		}
	}
	if enabled, ok := os.LookupEnv("CHANNEL_TEST_ENABLED"); ok {
		parsed, err := strconv.ParseBool(enabled)
		if err == nil {
			monitorSetting.AutoTestChannelEnabled = parsed
		}
	}
	monitorSetting.ChannelTestMode = NormalizeChannelTestMode(monitorSetting.ChannelTestMode)
	monitorSetting.ChannelTestConcurrency = NormalizeChannelTestConcurrency(monitorSetting.ChannelTestConcurrency)
	if monitorSetting.ModelHealthCheckMinutes < 1 || monitorSetting.ModelHealthCheckMinutes > MaxModelHealthCheckMinutes {
		monitorSetting.ModelHealthCheckMinutes = DefaultModelHealthCheckMinutes
	}
	return &monitorSetting
}

func NormalizeChannelTestMode(value string) string {
	switch strings.TrimSpace(value) {
	case ChannelTestModeAutoDetect, ChannelTestModeScheduledProbes:
		return ChannelTestModeAutoDetect
	case ChannelTestModeAutoDisable, ChannelTestModeAutoBanOnly:
		return ChannelTestModeAutoDisable
	case ChannelTestModePassiveRecovery:
		return ChannelTestModePassiveRecovery
	case ChannelTestModeScheduledAll:
		return ChannelTestModeScheduledAll
	default:
		return ChannelTestModeScheduledAll
	}
}

func NormalizeChannelTestConcurrency(concurrency int) int {
	if concurrency < 1 {
		return DefaultChannelTestConcurrency
	}
	if concurrency > MaxChannelTestConcurrency {
		return MaxChannelTestConcurrency
	}
	return concurrency
}

func ValidateChannelTestConcurrency(value string) error {
	concurrency, err := strconv.Atoi(value)
	if err != nil || concurrency < 1 || concurrency > MaxChannelTestConcurrency {
		return fmt.Errorf("channel test concurrency must be between 1 and %d", MaxChannelTestConcurrency)
	}
	return nil
}

func ValidateModelHealthCheckMinutes(value string) error {
	minutes, err := strconv.Atoi(value)
	if err != nil || minutes < 1 || minutes > MaxModelHealthCheckMinutes {
		return fmt.Errorf("model health check interval must be between 1 and %d minutes", MaxModelHealthCheckMinutes)
	}
	return nil
}

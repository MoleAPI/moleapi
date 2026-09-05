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
	ChannelTestType         string  `json:"channel_test_type"`
	ChannelTestCustomPrompt string  `json:"channel_test_custom_prompt"`
	ChannelTestCustomAnswer string  `json:"channel_test_custom_answer"`
}

const (
	ChannelTestModeScheduledAll    = "scheduled_all"
	ChannelTestModeAutoBanOnly     = "auto_ban_only"
	ChannelTestModePassiveRecovery = "passive_recovery"
	ChannelTestModeScheduledProbes = "scheduled_probes"

	ChannelTestConcurrencyOptionKey  = "monitor_setting.channel_test_concurrency"
	ChannelTestTypeOptionKey         = "monitor_setting.channel_test_type"
	ChannelTestCustomPromptOptionKey = "monitor_setting.channel_test_custom_prompt"
	ChannelTestCustomAnswerOptionKey = "monitor_setting.channel_test_custom_answer"
	DefaultChannelTestConcurrency    = 1
	MaxChannelTestConcurrency        = 32
	MaxChannelTestPromptLength       = 4000
	MaxChannelTestAnswerLength       = 500
)

// 默认配置
var monitorSetting = MonitorSetting{
	AutoTestChannelEnabled: false,
	AutoTestChannelMinutes: 10,
	ChannelTestMode:        ChannelTestModeScheduledAll,
	ChannelTestConcurrency: DefaultChannelTestConcurrency,
	ChannelTestType:        "hi",
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
	switch monitorSetting.ChannelTestMode {
	case ChannelTestModeAutoBanOnly, ChannelTestModePassiveRecovery, ChannelTestModeScheduledProbes:
	default:
		monitorSetting.ChannelTestMode = ChannelTestModeScheduledAll
	}
	monitorSetting.ChannelTestConcurrency = NormalizeChannelTestConcurrency(monitorSetting.ChannelTestConcurrency)
	monitorSetting.ChannelTestType = NormalizeChannelTestType(monitorSetting.ChannelTestType)
	return &monitorSetting
}

func NormalizeChannelTestType(value string) string {
	switch strings.TrimSpace(value) {
	case "intelligence", "custom":
		return strings.TrimSpace(value)
	default:
		return "hi"
	}
}

func ValidateChannelTestType(value string) error {
	value = strings.TrimSpace(value)
	if value != "hi" && value != "intelligence" && value != "custom" {
		return fmt.Errorf("channel test type must be hi, intelligence, or custom")
	}
	return nil
}

func ValidateChannelTestText(value string, maxLength int) error {
	if len([]rune(value)) > maxLength {
		return fmt.Errorf("channel test text must not exceed %d characters", maxLength)
	}
	return nil
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

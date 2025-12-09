package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// 项目级配置结构体，可按需扩展
type Config struct {
	MonitoringNamespace  string                  `json:"monitoringNamespace"`  // PrometheusRule / Dashboard 所在命名空间
	DefaultEnv           string                  `json:"defaultEnv"`           // 默认 env，例如 "dev"
	DashboardEnabled     bool                    `json:"dashboardEnabled"`     // 是否创建 Dashboard
	NetworkPolicyEnabled bool                    `json:"networkPolicyEnabled"` // 是否创建 NetworkPolicy
	QuotaProfiles        map[string]QuotaProfile `json:"quotaProfiles"`        // 不同环境的配额模板
}

type QuotaProfile struct {
	Name        string `json:"name"`
	CPURequests string `json:"cpuRequests"`
	CPULimits   string `json:"cpuLimits"`
	MemRequests string `json:"memRequests"`
	MemLimits   string `json:"memLimits"`
	Pods        string `json:"pods"`
}

// 默认配置，可选
func defaultConfig() *Config {
	return &Config{
		MonitoringNamespace:  "monitoring",
		DefaultEnv:           "dev",
		DashboardEnabled:     true,
		NetworkPolicyEnabled: false,
		QuotaProfiles: map[string]QuotaProfile{
			"dev": {
				Name:        "dev",
				CPURequests: "2",
				CPULimits:   "4",
				MemRequests: "4Gi",
				MemLimits:   "8Gi",
				Pods:        "50",
			},
			"prod": {
				Name:        "prod",
				CPURequests: "8",
				CPULimits:   "16",
				MemRequests: "32Gi",
				MemLimits:   "64Gi",
				Pods:        "200",
			},
		},
	}
}

// 简单从文件加载 JSON 配置（可按需换成 YAML）
func Load(path string) (*Config, error) {
	cfg := defaultConfig()
	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	return cfg, nil
}

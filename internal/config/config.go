package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

// Config represents the main configuration structure
type Config struct {
	Server     ServerConfig     `yaml:"server" mapstructure:"server"`
	Alicloud   AlicloudConfig   `yaml:"alicloud" mapstructure:"alicloud"`
	Services   ServicesConfig   `yaml:"services" mapstructure:"services"`
	Prometheus PrometheusConfig `yaml:"prometheus" mapstructure:"prometheus"`
}

// ServerConfig contains server-related configuration
type ServerConfig struct {
	ListenAddress string `yaml:"listen_address" mapstructure:"listen_address"`
	MetricsPath   string `yaml:"metrics_path" mapstructure:"metrics_path"`
	LogLevel      string `yaml:"log_level" mapstructure:"log_level"`
	LogFormat     string `yaml:"log_format" mapstructure:"log_format"`
}

// AlicloudConfig contains Alicloud-specific configuration
type AlicloudConfig struct {
	AccessKeyID     string          `yaml:"access_key_id" mapstructure:"access_key_id"`
	AccessKeySecret string          `yaml:"access_key_secret" mapstructure:"access_key_secret"`
	Region          string          `yaml:"region" mapstructure:"region"`
	Regions         []string        `yaml:"regions" mapstructure:"regions"`
	RateLimit       RateLimitConfig `yaml:"rate_limit" mapstructure:"rate_limit"`
}

// RateLimitConfig contains rate limiting configuration
type RateLimitConfig struct {
	RequestsPerSecond int `yaml:"requests_per_second" mapstructure:"requests_per_second"`
	Burst             int `yaml:"burst" mapstructure:"burst"`
}

// ServicesConfig contains configuration for all monitored services
type ServicesConfig struct {
	SLB   ServiceConfig `yaml:"slb" mapstructure:"slb"`
	Redis ServiceConfig `yaml:"redis" mapstructure:"redis"`
	RDS   ServiceConfig `yaml:"rds" mapstructure:"rds"`
}

// ServiceConfig contains configuration for a specific service
type ServiceConfig struct {
	Enabled        bool          `yaml:"enabled" mapstructure:"enabled"`
	Namespace      string        `yaml:"namespace" mapstructure:"namespace"`
	ScrapeInterval time.Duration `yaml:"scrape_interval" mapstructure:"scrape_interval"`
	Metrics        []string      `yaml:"metrics" mapstructure:"metrics"`
}

// PrometheusConfig contains Prometheus-specific configuration
type PrometheusConfig struct {
	GlobalLabels           map[string]string `yaml:"global_labels" mapstructure:"global_labels"`
	MetricPrefix           string            `yaml:"metric_prefix" mapstructure:"metric_prefix"`
	IncludeGoMetrics       bool              `yaml:"include_go_metrics" mapstructure:"include_go_metrics"`
	IncludeProcessMetrics  bool              `yaml:"include_process_metrics" mapstructure:"include_process_metrics"`
}

// Load loads configuration from file and environment variables
func Load(configPath string) (*Config, error) {
	v := viper.New()
	
	// Set default values
	setDefaults(v)
	
	// Configure viper
	v.SetConfigType("yaml")
	v.SetEnvPrefix("ALICLOUD_EXPORTER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	
	// Load config file if provided
	if configPath != "" {
		v.SetConfigFile(configPath)
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}
	
	// Unmarshal configuration
	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}
	
	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}
	
	return &config, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {
	v.SetDefault("server.listen_address", ":9100")
	v.SetDefault("server.metrics_path", "/metrics")
	v.SetDefault("server.log_level", "info")
	v.SetDefault("server.log_format", "json")
	
	v.SetDefault("alicloud.region", "cn-hangzhou")
	v.SetDefault("alicloud.rate_limit.requests_per_second", 10)
	v.SetDefault("alicloud.rate_limit.burst", 20)
	
	v.SetDefault("prometheus.metric_prefix", "alicloud")
	v.SetDefault("prometheus.include_go_metrics", false)
	v.SetDefault("prometheus.include_process_metrics", false)
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Alicloud.AccessKeyID == "" {
		return fmt.Errorf("alicloud.access_key_id is required")
	}
	if c.Alicloud.AccessKeySecret == "" {
		return fmt.Errorf("alicloud.access_key_secret is required")
	}
	if c.Alicloud.Region == "" {
		return fmt.Errorf("alicloud.region is required")
	}
	
	// Validate log level
	validLogLevels := []string{"debug", "info", "warn", "error"}
	if !contains(validLogLevels, c.Server.LogLevel) {
		return fmt.Errorf("invalid log level: %s, must be one of %v", c.Server.LogLevel, validLogLevels)
	}
	
	// Validate log format
	validLogFormats := []string{"json", "text"}
	if !contains(validLogFormats, c.Server.LogFormat) {
		return fmt.Errorf("invalid log format: %s, must be one of %v", c.Server.LogFormat, validLogFormats)
	}
	
	return nil
}

// SaveToFile saves the configuration to a YAML file
func (c *Config) SaveToFile(filename string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	
	return os.WriteFile(filename, data, 0644)
}

// contains checks if a slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// HTTPConfig HTTP服务器配置
type HTTPConfig struct {
	Port int `mapstructure:"port"`
}

// GrpcConfig gRPC服务器配置
type GrpcConfig struct {
	Port int `mapstructure:"port"`
}

// RedisConfig Redis连接配置
type RedisConfig struct {
	Addr                string `mapstructure:"addr"`
	Password            string `mapstructure:"password"`
	DB                  int    `mapstructure:"db"`
	PoolSize            int    `mapstructure:"pool_size"`
	DialTimeoutSeconds  int    `mapstructure:"dial_timeout_seconds"`
	ReadTimeoutSeconds  int    `mapstructure:"read_timeout_seconds"`
	WriteTimeoutSeconds int    `mapstructure:"write_timeout_seconds"`
}

// CaptchaConfigSpec 业务规则配置
type CaptchaConfigSpec struct {
	TTLSeconds      int `mapstructure:"ttl_seconds"`
	Width           int `mapstructure:"width"`
	Height          int `mapstructure:"height"`
	GraphSizeMin    int `mapstructure:"graph_size_min"`
	GraphSizeMax    int `mapstructure:"graph_size_max"`
	SliderTolerance int `mapstructure:"slider_tolerance"`
}

// TokenConfig Token配置
type TokenConfig struct {
	TTLSeconds int    `mapstructure:"ttl_seconds"`
	Secret     string `mapstructure:"secret"`
}

// NacosConfig Nacos服务注册配置
type NacosConfig struct {
	Enable      bool              `mapstructure:"enable"`
	ServerAddr  string            `mapstructure:"server_addr"`
	Namespace   string            `mapstructure:"namespace"`
	ServiceName string            `mapstructure:"service_name"`
	GroupName   string            `mapstructure:"group_name"`
	ClusterName string            `mapstructure:"cluster_name"`
	Weight      float64           `mapstructure:"weight"`
	Metadata    map[string]string `mapstructure:"metadata"`
}

// CaptchaConfig 全局配置结构
type CaptchaConfig struct {
	HTTP    HTTPConfig        `mapstructure:"http"`
	Grpc    GrpcConfig        `mapstructure:"grpc"`
	Redis   RedisConfig       `mapstructure:"redis"`
	Captcha CaptchaConfigSpec `mapstructure:"captcha"`
	Token   TokenConfig       `mapstructure:"token"`
	Nacos   NacosConfig       `mapstructure:"nacos"`
}

// LoadCaptchaConfig 加载配置文件
func LoadCaptchaConfig(configPath string) (*CaptchaConfig, error) {
	v := viper.New()
	v.SetConfigName("captcha")
	v.SetConfigType("yaml")
	v.AddConfigPath(configPath)

	if err := v.ReadInConfig(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg CaptchaConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	return &cfg, nil
}

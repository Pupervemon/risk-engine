package config

import "github.com/spf13/viper"

type CaptchaConfig struct {
	HTTP    HTTPConfig        `mapstructure:"http"`
	Grpc    GrpcConfig        `mapstructure:"grpc"`
	Redis   RedisConfig       `mapstructure:"redis"`
	Captcha CaptchaConfigSpec `mapstructure:"captcha"`
	Token   TokenConfig       `mapstructure:"token"`
}

type HTTPConfig struct {
	Port int `mapstructure:"port"`
}

type CaptchaConfigSpec struct {
	TTLSeconds      int `mapstructure:"ttl_seconds"`
	Width           int `mapstructure:"width"`
	Height          int `mapstructure:"height"`
	GraphSizeMin    int `mapstructure:"graph_size_min"`
	GraphSizeMax    int `mapstructure:"graph_size_max"`
	SliderTolerance int `mapstructure:"slider_tolerance"`
}

type TokenConfig struct {
	TTLSeconds int    `mapstructure:"ttl_seconds"`
	Secret     string `mapstructure:"secret"`
}

func LoadCaptchaConfig(path string) (*CaptchaConfig, error) {
	v := viper.New()
	v.AddConfigPath(path)
	v.SetConfigName("captcha")
	v.SetConfigType("yaml")
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		return nil, err
	}

	var cfg CaptchaConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

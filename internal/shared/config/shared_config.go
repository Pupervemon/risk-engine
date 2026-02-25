package config

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

// NacosConfig Nacos服务注册配置
type NacosConfig struct {
	Enable      bool              `mapstructure:"enable"`
	ServerAddr  string            `mapstructure:"server_addr"`
	Namespace   string            `mapstructure:"namespace"`
	ServiceName string            `mapstructure:"service_name"`
	GroupName   string            `mapstructure:"group_name"`
	ClusterName string            `mapstructure:"cluster_name"`
	RegisterIP  string            `mapstructure:"register_ip"`
	Weight      float64           `mapstructure:"weight"`
	Metadata    map[string]string `mapstructure:"metadata"`
}

package config

// HTTPConfig HTTP服务器配置
type HTTPConfig struct {
	// Port 是 HTTP 服务监听的端口号
	Port int `mapstructure:"port"`
}

// GrpcConfig gRPC服务器配置
type GrpcConfig struct {
	// Port 是 gRPC 服务监听的端口号
	Port int `mapstructure:"port"`
}

// RedisConfig Redis连接配置
type RedisConfig struct {
	// Addr 是 Redis 服务器的地址 (host:port)
	Addr string `mapstructure:"addr"`
	// Password 是 Redis 连接密码
	Password string `mapstructure:"password"`
	// DB 是 Redis 数据库索引
	DB int `mapstructure:"db"`
	// PoolSize 是 Redis 连接池的最大连接数
	PoolSize int `mapstructure:"pool_size"`
	// DialTimeoutSeconds 是建立连接的超时时间（秒）
	DialTimeoutSeconds int `mapstructure:"dial_timeout_seconds"`
	// ReadTimeoutSeconds 是读取操作的超时时间（秒）
	ReadTimeoutSeconds int `mapstructure:"read_timeout_seconds"`
	// WriteTimeoutSeconds 是写入操作的超时时间（秒）
	WriteTimeoutSeconds int `mapstructure:"write_timeout_seconds"`
}

// NacosConfig Nacos服务注册与配置中心设置
type NacosConfig struct {
	// Enable 是否启用 Nacos 注册中心
	Enable bool `mapstructure:"enable"`
	// ServerAddr Nacos 服务器地址
	ServerAddr string `mapstructure:"server_addr"`
	// Namespace Nacos 命名空间 ID
	Namespace string `mapstructure:"namespace"`
	// ServiceName 注册到 Nacos 的服务名称
	ServiceName string `mapstructure:"service_name"`
	// GroupName Nacos 服务分组名称
	GroupName string `mapstructure:"group_name"`
	// ClusterName Nacos 集群名称
	ClusterName string `mapstructure:"cluster_name"`
	// RegisterIP 显式指定的注册 IP（可选）
	RegisterIP string `mapstructure:"register_ip"`
	// Weight 服务权重
	Weight float64 `mapstructure:"weight"`
	// Metadata 服务元数据
	Metadata map[string]string `mapstructure:"metadata"`
}

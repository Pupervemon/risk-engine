package registry

import (
	"fmt"
	"net"
	"strconv"

	"github.com/nacos-group/nacos-sdk-go/v2/clients"
	"github.com/nacos-group/nacos-sdk-go/v2/clients/naming_client"
	"github.com/nacos-group/nacos-sdk-go/v2/common/constant"
	"github.com/nacos-group/nacos-sdk-go/v2/vo"
	"go.uber.org/zap"
)

// NacosConfig Nacos服务注册配置
type NacosConfig struct {
	ServerAddr  string            // Nacos服务器地址 (如: "127.0.0.1:8848")
	Namespace   string            // 命名空间
	ServiceName string            // 服务名称
	GroupName   string            // 分组名称
	ClusterName string            // 集群名称
	Weight      float64           // 服务权重
	Enable      bool              // 是否启用Nacos
	Metadata    map[string]string // 服务元数据
	ServicePort int               // 服务端口（用于健康检查）
	HealthCheck bool              // 是否启用健康检查
}

// NacosRegistry Nacos服务注册管理器
type NacosRegistry struct {
	client      naming_client.INamingClient
	config      *NacosConfig
	logger      *zap.Logger
	localIP     string
	isEphemeral bool // 是否为临时实例（默认true）
}

// NewNacosRegistry 创建Nacos注册中心客户端
func NewNacosRegistry(config *NacosConfig, logger *zap.Logger) (*NacosRegistry, error) {
	if !config.Enable {
		logger.Info("Nacos服务注册已禁用")
		return &NacosRegistry{
			config: config,
			logger: logger,
		}, nil
	}

	// 解析Nacos服务器地址
	host, portStr, err := net.SplitHostPort(config.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("解析Nacos服务器地址失败: %w", err)
	}
	port, err := strconv.ParseUint(portStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("解析Nacos端口失败: %w", err)
	}

	// 配置Nacos客户端
	serverConfigs := []constant.ServerConfig{
		*constant.NewServerConfig(host, port),
	}

	clientConfig := constant.NewClientConfig(
		constant.WithNamespaceId(config.Namespace),
		constant.WithTimeoutMs(5000),
		constant.WithNotLoadCacheAtStart(true),
		constant.WithLogDir("/tmp/nacos/log"),
		constant.WithCacheDir("/tmp/nacos/cache"),
		constant.WithLogLevel("info"),
	)

	// 创建Nacos客户端
	namingClient, err := clients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  clientConfig,
			ServerConfigs: serverConfigs,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("创建Nacos客户端失败: %w", err)
	}

	// 获取本地IP地址
	localIP, err := getLocalIP()
	if err != nil {
		return nil, fmt.Errorf("获取本地IP失败: %w", err)
	}

	logger.Info("Nacos注册中心初始化成功",
		zap.String("server_addr", config.ServerAddr),
		zap.String("namespace", config.Namespace),
		zap.String("local_ip", localIP))

	return &NacosRegistry{
		client:      namingClient,
		config:      config,
		logger:      logger,
		localIP:     localIP,
		isEphemeral: true, // 默认使用临时实例
	}, nil
}

// Register 注册服务到Nacos
func (nr *NacosRegistry) Register() error {
	if !nr.config.Enable {
		nr.logger.Info("Nacos未启用，跳过服务注册")
		return nil
	}

	success, err := nr.client.RegisterInstance(vo.RegisterInstanceParam{
		Ip:          nr.localIP,
		Port:        uint64(nr.config.ServicePort),
		ServiceName: nr.config.ServiceName,
		GroupName:   nr.config.GroupName,
		ClusterName: nr.config.ClusterName,
		Weight:      nr.config.Weight,
		Enable:      true,
		Healthy:     true,
		Ephemeral:   nr.isEphemeral,
		Metadata:    nr.config.Metadata,
	})

	if err != nil || !success {
		return fmt.Errorf("注册服务失败: %w", err)
	}

	nr.logger.Info("服务注册成功",
		zap.String("service_name", nr.config.ServiceName),
		zap.String("ip", nr.localIP),
		zap.Int("port", nr.config.ServicePort),
		zap.String("group", nr.config.GroupName),
		zap.String("cluster", nr.config.ClusterName))

	return nil
}

// Deregister 从Nacos注销服务
func (nr *NacosRegistry) Deregister() error {
	if !nr.config.Enable {
		nr.logger.Info("Nacos未启用，跳过服务注销")
		return nil
	}

	success, err := nr.client.DeregisterInstance(vo.DeregisterInstanceParam{
		Ip:          nr.localIP,
		Port:        uint64(nr.config.ServicePort),
		ServiceName: nr.config.ServiceName,
		GroupName:   nr.config.GroupName,
		Cluster:     nr.config.ClusterName,
		Ephemeral:   nr.isEphemeral,
	})

	if err != nil || !success {
		return fmt.Errorf("注销服务失败: %w", err)
	}

	nr.logger.Info("服务注销成功",
		zap.String("service_name", nr.config.ServiceName))

	return nil
}

// UpdateHealth 更新服务健康状态（可选）
func (nr *NacosRegistry) UpdateHealth(healthy bool) error {
	if !nr.config.Enable {
		return nil
	}

	// Nacos SDK v2会自动通过gRPC长连接发送心跳
	// 这个方法可用于手动更新健康状态（例如业务层主动上报）
	_, err := nr.client.UpdateInstance(vo.UpdateInstanceParam{
		Ip:          nr.localIP,
		Port:        uint64(nr.config.ServicePort),
		ServiceName: nr.config.ServiceName,
		GroupName:   nr.config.GroupName,
		ClusterName: nr.config.ClusterName,
		Weight:      nr.config.Weight,
		Enable:      true,
		Healthy:     healthy,
		Ephemeral:   nr.isEphemeral,
		Metadata:    nr.config.Metadata,
	})

	if err != nil {
		nr.logger.Error("更新健康状态失败",
			zap.Error(err),
			zap.Bool("healthy", healthy))
		return err
	}

	nr.logger.Debug("健康状态更新成功",
		zap.Bool("healthy", healthy))

	return nil
}

// getLocalIP 获取本地内网IP地址
func getLocalIP() (string, error) {
	// 获取所有网络接口
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("获取网络接口失败: %w", err)
	}

	// 遍历所有网络接口，找到第一个非回环的IPv4地址
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				// 优先选择私有IP段
				if isPrivateIP(ipNet.IP) {
					return ipNet.IP.String(), nil
				}
			}
		}
	}

	// 如果没有找到私有IP，再次遍历找任意IPv4地址
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("未找到有效的IP地址")
}

// isPrivateIP 判断是否为私有IP地址
func isPrivateIP(ip net.IP) bool {
	// 私有IP段:
	// 10.0.0.0/8
	// 172.16.0.0/12
	// 192.168.0.0/16
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
			(ip4[0] == 192 && ip4[1] == 168)
	}
	return false
}

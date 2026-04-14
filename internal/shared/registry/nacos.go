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

// NacosConfig describes service registration settings for Nacos.
type NacosConfig struct {
	ServerAddr  string
	Namespace   string
	ServiceName string
	GroupName   string
	ClusterName string
	RegisterIP  string
	Weight      float64
	Enable      bool
	Metadata    map[string]string
	HttpPort    int
	GrpcPort    int
	HealthCheck bool
}

// NacosRegistry manages registration and deregistration against Nacos.
type NacosRegistry struct {
	client      naming_client.INamingClient
	config      *NacosConfig
	logger      *zap.Logger
	localIP     string
	isEphemeral bool
}

// NewNacosRegistry creates a new Nacos registry client.
func NewNacosRegistry(config *NacosConfig, logger *zap.Logger) (*NacosRegistry, error) {
	if !config.Enable {
		logger.Info("nacos service registration disabled")
		return &NacosRegistry{
			config: config,
			logger: logger,
		}, nil
	}

	if config.Metadata == nil {
		config.Metadata = make(map[string]string)
	}
	if config.HttpPort > 0 {
		config.Metadata["http-port"] = strconv.Itoa(config.HttpPort)
	}
	if config.GrpcPort > 0 {
		config.Metadata["grpc-port"] = strconv.Itoa(config.GrpcPort)
	}

	host, portStr, err := net.SplitHostPort(config.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("parse nacos server address: %w", err)
	}
	port, err := strconv.ParseUint(portStr, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse nacos port: %w", err)
	}

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

	namingClient, err := clients.NewNamingClient(
		vo.NacosClientParam{
			ClientConfig:  clientConfig,
			ServerConfigs: serverConfigs,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("create nacos client: %w", err)
	}

	localIP, err := resolveRegisterIP(config.RegisterIP)
	if err != nil {
		return nil, err
	}

	logger.Info("nacos registry initialized",
		zap.String("server_addr", config.ServerAddr),
		zap.String("namespace", config.Namespace),
		zap.String("local_ip", localIP))

	return &NacosRegistry{
		client:      namingClient,
		config:      config,
		logger:      logger,
		localIP:     localIP,
		isEphemeral: true,
	}, nil
}

func resolveRegisterIP(overrideIP string) (string, error) {
	if overrideIP != "" {
		parsed := net.ParseIP(overrideIP)
		if parsed == nil || parsed.To4() == nil {
			return "", fmt.Errorf("invalid register_ip: %s", overrideIP)
		}
		return parsed.String(), nil
	}

	ip, err := getLocalIP()
	if err != nil {
		return "", fmt.Errorf("resolve local IP: %w", err)
	}

	return ip, nil
}

// Register registers the service instance with Nacos.
func (nr *NacosRegistry) Register() error {
	if !nr.config.Enable {
		nr.logger.Info("nacos disabled, skipping service registration")
		return nil
	}

	registerPort := nr.config.HttpPort
	if registerPort == 0 {
		registerPort = nr.config.GrpcPort
	}

	success, err := nr.client.RegisterInstance(vo.RegisterInstanceParam{
		Ip:          nr.localIP,
		Port:        uint64(registerPort),
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
		return fmt.Errorf("register service: %w", err)
	}

	nr.logger.Info("service registered",
		zap.String("service_name", nr.config.ServiceName),
		zap.String("ip", nr.localIP),
		zap.Int("port", registerPort),
		zap.String("group", nr.config.GroupName),
		zap.String("cluster", nr.config.ClusterName))

	return nil
}

// Deregister removes the service instance from Nacos.
func (nr *NacosRegistry) Deregister() error {
	if !nr.config.Enable {
		nr.logger.Info("nacos disabled, skipping service deregistration")
		return nil
	}

	registerPort := nr.config.HttpPort
	if registerPort == 0 {
		registerPort = nr.config.GrpcPort
	}

	success, err := nr.client.DeregisterInstance(vo.DeregisterInstanceParam{
		Ip:          nr.localIP,
		Port:        uint64(registerPort),
		ServiceName: nr.config.ServiceName,
		GroupName:   nr.config.GroupName,
		Cluster:     nr.config.ClusterName,
		Ephemeral:   nr.isEphemeral,
	})

	if err != nil || !success {
		return fmt.Errorf("deregister service: %w", err)
	}

	nr.logger.Info("service deregistered",
		zap.String("service_name", nr.config.ServiceName))

	return nil
}

// UpdateHealth updates the instance health state in Nacos.
func (nr *NacosRegistry) UpdateHealth(healthy bool) error {
	if !nr.config.Enable {
		return nil
	}

	registerPort := nr.config.HttpPort
	if registerPort == 0 {
		registerPort = nr.config.GrpcPort
	}

	_, err := nr.client.UpdateInstance(vo.UpdateInstanceParam{
		Ip:          nr.localIP,
		Port:        uint64(registerPort),
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
		nr.logger.Error("failed to update health state",
			zap.Error(err),
			zap.Bool("healthy", healthy))
		return err
	}

	nr.logger.Debug("health state updated",
		zap.Bool("healthy", healthy))

	return nil
}

// getLocalIP returns a non-loopback IPv4 address for registration.
func getLocalIP() (string, error) {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "", fmt.Errorf("list network interfaces: %w", err)
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				if isPrivateIP(ipNet.IP) {
					return ipNet.IP.String(), nil
				}
			}
		}
	}

	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && !ipNet.IP.IsLoopback() {
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no valid IPv4 address found")
}

// isPrivateIP reports whether the address belongs to a private IPv4 range.
func isPrivateIP(ip net.IP) bool {
	if ip4 := ip.To4(); ip4 != nil {
		return ip4[0] == 10 ||
			(ip4[0] == 172 && ip4[1] >= 16 && ip4[1] <= 31) ||
			(ip4[0] == 192 && ip4[1] == 168)
	}
	return false
}

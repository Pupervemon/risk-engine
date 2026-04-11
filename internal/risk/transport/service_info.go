package transport

import "fmt"

// ServiceInfo describes the runtime service metadata exposed by helper endpoints.
type ServiceInfo struct {
	Name        string
	Version     string
	Protocol    string
	Description string
	HTTPPort    int
	GRPCPort    int
}

func (s ServiceInfo) normalized() ServiceInfo {
	if s.Name == "" {
		s.Name = "risk-service"
	}
	if s.Version == "" {
		s.Version = "1.0.0"
	}
	if s.Protocol == "" {
		s.Protocol = "grpc"
	}
	if s.Description == "" {
		s.Description = "Risk Engine - 风控引擎服务"
	}
	return s
}

func (s ServiceInfo) grpcEndpoint() string {
	if s.GRPCPort <= 0 {
		return "disabled"
	}
	return fmt.Sprintf("port %d", s.GRPCPort)
}

func (s ServiceInfo) httpEndpoint() string {
	if s.HTTPPort <= 0 {
		return "disabled"
	}
	return fmt.Sprintf("port %d", s.HTTPPort)
}

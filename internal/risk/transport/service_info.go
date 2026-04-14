package transport

import "fmt"

// ServiceInfo describes the runtime service metadata exposed by helper endpoints.
// Empty or missing fields are rendered as unknown so callers can clearly see
// that the metadata was not configured.
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
		s.Name = "unknown"
	}
	if s.Version == "" {
		s.Version = "unknown"
	}
	if s.Protocol == "" {
		s.Protocol = "unknown"
	}
	if s.Description == "" {
		s.Description = "unknown"
	}
	return s
}

func (s ServiceInfo) grpcEndpoint() string {
	if s.GRPCPort <= 0 {
		return "unknown"
	}
	return fmt.Sprintf("port %d", s.GRPCPort)
}

func (s ServiceInfo) httpEndpoint() string {
	if s.HTTPPort <= 0 {
		return "unknown"
	}
	return fmt.Sprintf("port %d", s.HTTPPort)
}

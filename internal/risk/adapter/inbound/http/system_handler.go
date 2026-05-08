package http

import (
	"net/http"
	"time"

	"github.com/Pupervemon/risk-engine/internal/shared/health"
	"github.com/gin-gonic/gin"
)

// ErrorResponse is the standard error payload for transport-layer failures.
type ErrorResponse struct {
	Error string `json:"error"`
}

// ServiceEndpointsResponse documents the exposed HTTP and gRPC entrypoints.
type ServiceEndpointsResponse struct {
	HTTP        string `json:"http"`
	GRPC        string `json:"grpc"`
	Health      string `json:"health"`
	AdminRiskIP string `json:"admin_riskip,omitempty"`
}

// ServiceInfoResponse documents the payload returned by the /info endpoint.
type ServiceInfoResponse struct {
	Service     string                   `json:"service"`
	Version     string                   `json:"version"`
	Protocol    string                   `json:"protocol"`
	Description string                   `json:"description"`
	Endpoints   ServiceEndpointsResponse `json:"endpoints"`
}

// RiskSystemHandler serves health-check and service metadata endpoints.
type RiskSystemHandler struct {
	healthChecker *health.Checker
	serviceInfo   ServiceInfo
}

// NewRiskSystemHandler creates a system endpoint handler set for the risk service.
func NewRiskSystemHandler(healthChecker *health.Checker, serviceInfo ServiceInfo) *RiskSystemHandler {
	return &RiskSystemHandler{
		healthChecker: healthChecker,
		serviceInfo:   serviceInfo.normalized(),
	}
}

// Health godoc
// @Summary Service health check
// @Description Returns overall service health and dependency status.
// @Tags Health
// @Produce json
// @Success 200 {object} HealthResponseDoc
// @Failure 503 {object} HealthResponseDoc
// @Router /health [get]
func (h *RiskSystemHandler) Health(c *gin.Context) {
	h.writeHealth(c)
}

// Info godoc
// @Summary Service metadata
// @Description Returns service name, version, protocol, and exposed endpoints.
// @Tags Service Info
// @Produce json
// @Success 200 {object} ServiceInfoResponse
// @Router /info [get]
func (h *RiskSystemHandler) Info(c *gin.Context) {
	c.JSON(http.StatusOK, ServiceInfoResponse{
		Service:     h.serviceInfo.Name,
		Version:     h.serviceInfo.Version,
		Protocol:    h.serviceInfo.Protocol,
		Description: h.serviceInfo.Description,
		Endpoints: ServiceEndpointsResponse{
			HTTP:        h.serviceInfo.httpEndpoint(),
			GRPC:        h.serviceInfo.grpcEndpoint(),
			Health:      "/health",
			AdminRiskIP: "/api/v1/admin/risk-ips, /api/v1/admin/risk-ips/{ip}, /api/v1/admin/risk-ips/{ip}/events",
		},
	})
}

func (h *RiskSystemHandler) writeHealth(c *gin.Context) {
	response := health.HealthResponse{
		Status:     health.StatusUP,
		Components: map[string]health.ComponentCheck{},
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	redisCheck := h.healthChecker.CheckRedis(c.Request.Context())
	response.Components["redis"] = redisCheck
	if redisCheck.Status == health.StatusDOWN {
		response.Status = health.StatusDOWN
		c.JSON(http.StatusServiceUnavailable, response)
		return
	}

	c.JSON(http.StatusOK, response)
}

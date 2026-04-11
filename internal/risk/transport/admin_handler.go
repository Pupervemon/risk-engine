package transport

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"

	riskservice "github.com/Pupervemon/risk-engine/internal/risk/service"
	"github.com/gin-gonic/gin"
)

type RiskAdminReader interface {
	ListRiskIPs(ctx context.Context, query riskservice.RiskIPListQuery) (*riskservice.RiskIPListResponse, error)
	GetRiskIP(ctx context.Context, ip string) (*riskservice.RiskIPDetail, error)
	ListRiskIPEvents(ctx context.Context, ip string, query riskservice.RiskIPEventsQuery) (*riskservice.RiskIPEventsResponse, error)
}

type RiskAdminHandler struct {
	Reader RiskAdminReader
}

func NewRiskAdminHandler(reader RiskAdminReader) *RiskAdminHandler {
	return &RiskAdminHandler{Reader: reader}
}

func (h *RiskAdminHandler) ListRiskIPs(c *gin.Context) {
	limit, ok := parseOptionalIntQuery(c, "limit")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_LIMIT"})
		return
	}

	offset, ok := parseOptionalIntQuery(c, "offset")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_OFFSET"})
		return
	}

	resp, err := h.Reader.ListRiskIPs(c.Request.Context(), riskservice.RiskIPListQuery{
		Limit:  limit,
		Offset: offset,
		Search: strings.TrimSpace(c.Query("q")),
	})
	if err != nil {
		switch {
		case errors.Is(err, riskservice.ErrInvalidRiskIP):
			c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_IP"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "LIST_RISK_IPS_FAILED"})
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *RiskAdminHandler) GetRiskIP(c *gin.Context) {
	resp, err := h.Reader.GetRiskIP(c.Request.Context(), c.Param("ip"))
	if err != nil {
		switch {
		case errors.Is(err, riskservice.ErrInvalidRiskIP):
			c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_IP"})
		case errors.Is(err, riskservice.ErrRiskIPNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "RISK_IP_NOT_FOUND"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "GET_RISK_IP_FAILED"})
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *RiskAdminHandler) GetRiskIPEvents(c *gin.Context) {
	limit, ok := parseOptionalIntQuery(c, "limit")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_LIMIT"})
		return
	}

	offset, ok := parseOptionalIntQuery(c, "offset")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_OFFSET"})
		return
	}

	resp, err := h.Reader.ListRiskIPEvents(c.Request.Context(), c.Param("ip"), riskservice.RiskIPEventsQuery{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		switch {
		case errors.Is(err, riskservice.ErrInvalidRiskIP):
			c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_IP"})
		case errors.Is(err, riskservice.ErrRiskIPNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "RISK_IP_NOT_FOUND"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "GET_RISK_IP_EVENTS_FAILED"})
		}
		return
	}

	c.JSON(http.StatusOK, resp)
}

func parseOptionalIntQuery(c *gin.Context, key string) (int, bool) {
	raw := strings.TrimSpace(c.Query(key))
	if raw == "" {
		return 0, true
	}

	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0, false
	}
	return value, true
}

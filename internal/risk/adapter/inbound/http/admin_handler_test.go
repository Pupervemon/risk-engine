package http

import (
	"context"
	"encoding/json"
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Pupervemon/risk-engine/internal/risk/application/ports"
	"github.com/Pupervemon/risk-engine/internal/risk/domain"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

type fakeRiskInsightQuery struct {
	listQuery   ports.RiskIPListQuery
	listResp    *ports.RiskIPListResponse
	listErr     error
	getIP       string
	getResp     *domain.RiskIPDetail
	getErr      error
	eventsIP    string
	eventsQuery ports.RiskIPEventsQuery
	eventsResp  *ports.RiskIPEventsResponse
	eventsErr   error
}

func (f *fakeRiskInsightQuery) ListRiskIPs(_ context.Context, query ports.RiskIPListQuery) (*ports.RiskIPListResponse, error) {
	f.listQuery = query
	return f.listResp, f.listErr
}

func (f *fakeRiskInsightQuery) GetRiskIP(_ context.Context, ip string) (*domain.RiskIPDetail, error) {
	f.getIP = ip
	return f.getResp, f.getErr
}

func (f *fakeRiskInsightQuery) ListRiskIPEvents(_ context.Context, ip string, query ports.RiskIPEventsQuery) (*ports.RiskIPEventsResponse, error) {
	f.eventsIP = ip
	f.eventsQuery = query
	return f.eventsResp, f.eventsErr
}

func TestRiskAdminRoutesRequireGatewayPrincipal(t *testing.T) {
	router := newRiskAdminTestRouter(&fakeRiskInsightQuery{})

	recorder := performRiskAdminRequest(router, stdhttp.MethodGet, "/api/v1/admin/risk-ips", nil)
	assertHTTPStatus(t, recorder, stdhttp.StatusUnauthorized)
	assertErrorBody(t, recorder.Body.String(), "UNAUTHORIZED")
}

func TestRiskAdminRoutesRejectInsufficientRole(t *testing.T) {
	router := newRiskAdminTestRouter(&fakeRiskInsightQuery{})
	headers := stdhttp.Header{
		standardUserIDHeader:    []string{"42"},
		standardUserRolesHeader: []string{"1"},
	}

	recorder := performRiskAdminRequest(router, stdhttp.MethodGet, "/api/v1/admin/risk-ips", headers)
	assertHTTPStatus(t, recorder, stdhttp.StatusForbidden)
	assertErrorBody(t, recorder.Body.String(), "FORBIDDEN")
}

func TestListRiskIPsSuccess(t *testing.T) {
	reader := &fakeRiskInsightQuery{
		listResp: &ports.RiskIPListResponse{
			Items: []domain.RiskIPSummary{
				{
					IP:           "127.0.0.1",
					Severity:     "medium",
					TotalChecks:  3,
					TotalRejects: 1,
				},
			},
			Offset:  5,
			Limit:   10,
			Total:   1,
			HasMore: false,
		},
	}
	router := newRiskAdminTestRouter(reader)

	recorder := performRiskAdminRequest(router, stdhttp.MethodGet, "/api/v1/admin/risk-ips?limit=10&offset=5&q=+127.0.0.1+", adminHeaders(RoleTeacher))
	assertHTTPStatus(t, recorder, stdhttp.StatusOK)

	if reader.listQuery.Limit != 10 || reader.listQuery.Offset != 5 || reader.listQuery.Search != "127.0.0.1" {
		t.Fatalf("unexpected list query: %+v", reader.listQuery)
	}
	var body ports.RiskIPListResponse
	decodeJSONBody(t, recorder.Body.String(), &body)
	if body.Total != 1 || len(body.Items) != 1 || body.Items[0].IP != "127.0.0.1" || body.Items[0].Severity != "medium" {
		t.Fatalf("unexpected list response body: %+v", body)
	}
}

func TestListRiskIPsValidatesPagination(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		statusCode int
		errorCode  string
	}{
		{name: "invalid limit", path: "/api/v1/admin/risk-ips?limit=-1", statusCode: stdhttp.StatusBadRequest, errorCode: "INVALID_LIMIT"},
		{name: "invalid offset", path: "/api/v1/admin/risk-ips?offset=abc", statusCode: stdhttp.StatusBadRequest, errorCode: "INVALID_OFFSET"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newRiskAdminTestRouter(&fakeRiskInsightQuery{})

			recorder := performRiskAdminRequest(router, stdhttp.MethodGet, tt.path, adminHeaders(RoleAdmin))
			assertHTTPStatus(t, recorder, tt.statusCode)
			assertErrorBody(t, recorder.Body.String(), tt.errorCode)
		})
	}
}

func TestListRiskIPsMapsReaderErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		errorCode  string
	}{
		{name: "invalid ip", err: domain.ErrInvalidRiskIP, statusCode: stdhttp.StatusBadRequest, errorCode: "INVALID_IP"},
		{name: "backend failure", err: errors.New("redis down"), statusCode: stdhttp.StatusInternalServerError, errorCode: "LIST_RISK_IPS_FAILED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newRiskAdminTestRouter(&fakeRiskInsightQuery{listErr: tt.err})

			recorder := performRiskAdminRequest(router, stdhttp.MethodGet, "/api/v1/admin/risk-ips?q=bad-ip", adminHeaders(RoleTeacher))
			assertHTTPStatus(t, recorder, tt.statusCode)
			assertErrorBody(t, recorder.Body.String(), tt.errorCode)
		})
	}
}

func TestGetRiskIPSuccess(t *testing.T) {
	reader := &fakeRiskInsightQuery{
		getResp: &domain.RiskIPDetail{
			RiskIPSummary: domain.RiskIPSummary{
				IP:          "127.0.0.1",
				Severity:    "critical",
				Blacklisted: true,
			},
			EventCount: 2,
		},
	}
	router := newRiskAdminTestRouter(reader)

	recorder := performRiskAdminRequest(router, stdhttp.MethodGet, "/api/v1/admin/risk-ips/127.0.0.1", adminHeaders(RoleAdmin))
	assertHTTPStatus(t, recorder, stdhttp.StatusOK)

	if reader.getIP != "127.0.0.1" {
		t.Fatalf("expected queried IP 127.0.0.1, got %q", reader.getIP)
	}
	var body domain.RiskIPDetail
	decodeJSONBody(t, recorder.Body.String(), &body)
	if body.IP != "127.0.0.1" || body.Severity != "critical" || body.EventCount != 2 {
		t.Fatalf("unexpected detail response body: %+v", body)
	}
}

func TestGetRiskIPMapsReaderErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		errorCode  string
	}{
		{name: "invalid ip", err: domain.ErrInvalidRiskIP, statusCode: stdhttp.StatusBadRequest, errorCode: "INVALID_IP"},
		{name: "not found", err: domain.ErrRiskIPNotFound, statusCode: stdhttp.StatusNotFound, errorCode: "RISK_IP_NOT_FOUND"},
		{name: "backend failure", err: errors.New("redis down"), statusCode: stdhttp.StatusInternalServerError, errorCode: "GET_RISK_IP_FAILED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newRiskAdminTestRouter(&fakeRiskInsightQuery{getErr: tt.err})

			recorder := performRiskAdminRequest(router, stdhttp.MethodGet, "/api/v1/admin/risk-ips/127.0.0.1", adminHeaders(RoleTeacher))
			assertHTTPStatus(t, recorder, tt.statusCode)
			assertErrorBody(t, recorder.Body.String(), tt.errorCode)
		})
	}
}

func TestListRiskIPEventsSuccess(t *testing.T) {
	reader := &fakeRiskInsightQuery{
		eventsResp: &ports.RiskIPEventsResponse{
			IP: "127.0.0.1",
			Items: []domain.RiskIPEvent{
				{
					EventID:    "event-1",
					EventType:  "check",
					IP:         "127.0.0.1",
					Action:     "reject",
					OccurredAt: "2026-05-08T00:00:00Z",
				},
			},
			Offset:  3,
			Limit:   20,
			Total:   1,
			HasMore: false,
		},
	}
	router := newRiskAdminTestRouter(reader)

	recorder := performRiskAdminRequest(router, stdhttp.MethodGet, "/api/v1/admin/risk-ips/127.0.0.1/events?limit=20&offset=3", adminHeaders(RoleTeacher))
	assertHTTPStatus(t, recorder, stdhttp.StatusOK)

	if reader.eventsIP != "127.0.0.1" || reader.eventsQuery.Limit != 20 || reader.eventsQuery.Offset != 3 {
		t.Fatalf("unexpected events query: ip=%q query=%+v", reader.eventsIP, reader.eventsQuery)
	}
	var body ports.RiskIPEventsResponse
	decodeJSONBody(t, recorder.Body.String(), &body)
	if body.IP != "127.0.0.1" || len(body.Items) != 1 || body.Items[0].EventID != "event-1" {
		t.Fatalf("unexpected events response body: %+v", body)
	}
}

func TestListRiskIPEventsValidatesPagination(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		errorCode string
	}{
		{name: "invalid limit", path: "/api/v1/admin/risk-ips/127.0.0.1/events?limit=nope", errorCode: "INVALID_LIMIT"},
		{name: "invalid offset", path: "/api/v1/admin/risk-ips/127.0.0.1/events?offset=-1", errorCode: "INVALID_OFFSET"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newRiskAdminTestRouter(&fakeRiskInsightQuery{})

			recorder := performRiskAdminRequest(router, stdhttp.MethodGet, tt.path, adminHeaders(RoleAdmin))
			assertHTTPStatus(t, recorder, stdhttp.StatusBadRequest)
			assertErrorBody(t, recorder.Body.String(), tt.errorCode)
		})
	}
}

func TestListRiskIPEventsMapsReaderErrors(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		errorCode  string
	}{
		{name: "invalid ip", err: domain.ErrInvalidRiskIP, statusCode: stdhttp.StatusBadRequest, errorCode: "INVALID_IP"},
		{name: "not found", err: domain.ErrRiskIPNotFound, statusCode: stdhttp.StatusNotFound, errorCode: "RISK_IP_NOT_FOUND"},
		{name: "backend failure", err: errors.New("redis down"), statusCode: stdhttp.StatusInternalServerError, errorCode: "GET_RISK_IP_EVENTS_FAILED"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := newRiskAdminTestRouter(&fakeRiskInsightQuery{eventsErr: tt.err})

			recorder := performRiskAdminRequest(router, stdhttp.MethodGet, "/api/v1/admin/risk-ips/127.0.0.1/events", adminHeaders(RoleTeacher))
			assertHTTPStatus(t, recorder, tt.statusCode)
			assertErrorBody(t, recorder.Body.String(), tt.errorCode)
		})
	}
}

func newRiskAdminTestRouter(reader ports.RiskInsightQuery) *gin.Engine {
	gin.SetMode(gin.TestMode)
	return NewRiskRouter(goredis.NewClient(&goredis.Options{Addr: "127.0.0.1:0"}), zap.NewNop(), ServiceInfo{
		Name:        "risk-service",
		Version:     "test",
		Protocol:    "grpc",
		Description: "test risk service",
		HTTPPort:    9080,
		GRPCPort:    9090,
	}, reader)
}

func performRiskAdminRequest(router *gin.Engine, method string, path string, headers stdhttp.Header) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, path, nil)
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	router.ServeHTTP(recorder, request)
	return recorder
}

func adminHeaders(role int) stdhttp.Header {
	return stdhttp.Header{
		standardUserIDHeader:    []string{"42"},
		standardUserRolesHeader: []string{string(rune('0' + role))},
	}
}

func assertHTTPStatus(t *testing.T, recorder *httptest.ResponseRecorder, expected int) {
	t.Helper()

	if recorder.Code != expected {
		t.Fatalf("expected HTTP status %d, got %d body=%s", expected, recorder.Code, recorder.Body.String())
	}
}

func assertErrorBody(t *testing.T, raw string, expected string) {
	t.Helper()

	var body struct {
		Error string `json:"error"`
	}
	decodeJSONBody(t, raw, &body)
	if body.Error != expected {
		t.Fatalf("expected error %q, got %q from body %s", expected, body.Error, raw)
	}
}

func decodeJSONBody(t *testing.T, raw string, target interface{}) {
	t.Helper()

	decoder := json.NewDecoder(strings.NewReader(raw))
	if err := decoder.Decode(target); err != nil {
		t.Fatalf("failed to decode JSON body %q: %v", raw, err)
	}
}

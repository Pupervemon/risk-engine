package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseRequestPrincipalAcceptsStandardHeaders(t *testing.T) {
	t.Parallel()

	principal, err := parseRequestPrincipal(newRiskTestContext(http.Header{
		standardUserIDHeader:    []string{"42"},
		standardUserRolesHeader: []string{"2,3"},
	}))
	if err != nil {
		t.Fatalf("parseRequestPrincipal returned error: %v", err)
	}
	if principal.UserID != "42" {
		t.Fatalf("expected user id 42, got %q", principal.UserID)
	}
	if !sameRoleSet(principal.Roles, []int{2, 3}) {
		t.Fatalf("expected roles [2 3], got %v", principal.Roles)
	}
}

func TestParseRequestPrincipalAcceptsLegacyHeaders(t *testing.T) {
	t.Parallel()

	principal, err := parseRequestPrincipal(newRiskTestContext(http.Header{
		gatewayUserIDHeader:    []string{"7"},
		gatewayUserRolesHeader: []string{`["3","2"]`},
	}))
	if err != nil {
		t.Fatalf("parseRequestPrincipal returned error: %v", err)
	}
	if principal.UserID != "7" {
		t.Fatalf("expected user id 7, got %q", principal.UserID)
	}
	if !sameRoleSet(principal.Roles, []int{2, 3}) {
		t.Fatalf("expected roles [2 3], got %v", principal.Roles)
	}
}

func TestParseRequestPrincipalAcceptsEquivalentHeadersAcrossAliases(t *testing.T) {
	t.Parallel()

	principal, err := parseRequestPrincipal(newRiskTestContext(http.Header{
		standardUserIDHeader:    []string{"99"},
		gatewayUserIDHeader:     []string{"99"},
		standardUserRolesHeader: []string{"2,3"},
		gatewayUserRolesHeader:  []string{`[3,2]`},
	}))
	if err != nil {
		t.Fatalf("parseRequestPrincipal returned error: %v", err)
	}
	if principal.UserID != "99" {
		t.Fatalf("expected user id 99, got %q", principal.UserID)
	}
	if !sameRoleSet(principal.Roles, []int{2, 3}) {
		t.Fatalf("expected roles [2 3], got %v", principal.Roles)
	}
}

func TestParseRequestPrincipalRejectsConflictingUserIDs(t *testing.T) {
	t.Parallel()

	_, err := parseRequestPrincipal(newRiskTestContext(http.Header{
		standardUserIDHeader:    []string{"42"},
		gatewayUserIDHeader:     []string{"43"},
		standardUserRolesHeader: []string{"3"},
	}))
	if err == nil {
		t.Fatal("expected conflicting user ids to be rejected")
	}
}

func TestParseRequestPrincipalRejectsConflictingRoleSets(t *testing.T) {
	t.Parallel()

	_, err := parseRequestPrincipal(newRiskTestContext(http.Header{
		standardUserIDHeader:    []string{"42"},
		standardUserRolesHeader: []string{"2,3"},
		gatewayUserRolesHeader:  []string{"3"},
	}))
	if err == nil {
		t.Fatal("expected conflicting role sets to be rejected")
	}
}

func newRiskTestContext(headers http.Header) *gin.Context {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header = make(http.Header, len(headers))
	for key, values := range headers {
		for _, value := range values {
			request.Header.Add(key, value)
		}
	}
	context.Request = request

	return context
}

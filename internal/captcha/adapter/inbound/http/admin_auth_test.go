package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseAdminPrincipalAcceptsStandardHeaders(t *testing.T) {
	t.Parallel()

	principal, err := parseAdminPrincipal(newAdminTestContext(http.Header{
		captchaGatewayStandardUserIDHeader:    []string{"42"},
		captchaGatewayStandardUserRolesHeader: []string{"1,3"},
	}))
	if err != nil {
		t.Fatalf("parseAdminPrincipal returned error: %v", err)
	}
	if principal.UserID != "42" {
		t.Fatalf("expected user id 42, got %q", principal.UserID)
	}
	if !sameAdminRoleSet(principal.Roles, []int{1, 3}) {
		t.Fatalf("expected roles [1 3], got %v", principal.Roles)
	}
}

func TestParseAdminPrincipalAcceptsLegacyHeaders(t *testing.T) {
	t.Parallel()

	principal, err := parseAdminPrincipal(newAdminTestContext(http.Header{
		captchaGatewayUserIDHeader:    []string{"7"},
		captchaGatewayUserRolesHeader: []string{`["3","1"]`},
	}))
	if err != nil {
		t.Fatalf("parseAdminPrincipal returned error: %v", err)
	}
	if principal.UserID != "7" {
		t.Fatalf("expected user id 7, got %q", principal.UserID)
	}
	if !sameAdminRoleSet(principal.Roles, []int{1, 3}) {
		t.Fatalf("expected roles [1 3], got %v", principal.Roles)
	}
}

func TestParseAdminPrincipalAcceptsEquivalentHeadersAcrossAliases(t *testing.T) {
	t.Parallel()

	principal, err := parseAdminPrincipal(newAdminTestContext(http.Header{
		captchaGatewayStandardUserIDHeader:    []string{"99"},
		captchaGatewayUserIDHeader:            []string{"99"},
		captchaGatewayStandardUserRolesHeader: []string{"1,3"},
		captchaGatewayUserRolesHeader:         []string{`[3,1]`},
	}))
	if err != nil {
		t.Fatalf("parseAdminPrincipal returned error: %v", err)
	}
	if principal.UserID != "99" {
		t.Fatalf("expected user id 99, got %q", principal.UserID)
	}
	if !sameAdminRoleSet(principal.Roles, []int{1, 3}) {
		t.Fatalf("expected roles [1 3], got %v", principal.Roles)
	}
}

func TestParseAdminPrincipalRejectsConflictingUserIDs(t *testing.T) {
	t.Parallel()

	_, err := parseAdminPrincipal(newAdminTestContext(http.Header{
		captchaGatewayStandardUserIDHeader:    []string{"42"},
		captchaGatewayUserIDHeader:            []string{"43"},
		captchaGatewayStandardUserRolesHeader: []string{"3"},
	}))
	if err == nil {
		t.Fatal("expected conflicting user ids to be rejected")
	}
}

func TestParseAdminPrincipalRejectsConflictingRoleSets(t *testing.T) {
	t.Parallel()

	_, err := parseAdminPrincipal(newAdminTestContext(http.Header{
		captchaGatewayStandardUserIDHeader:    []string{"42"},
		captchaGatewayStandardUserRolesHeader: []string{"1,3"},
		captchaGatewayUserRolesHeader:         []string{"3"},
	}))
	if err == nil {
		t.Fatal("expected conflicting role sets to be rejected")
	}
}

func newAdminTestContext(headers http.Header) *gin.Context {
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

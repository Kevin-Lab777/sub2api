package routes

import (
	"strings"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/handler"
	"github.com/Wei-Shaw/sub2api/internal/server/middleware"
	"github.com/gin-gonic/gin"
)

func TestAdminIdentityRoutesExcludeCustomerSelfService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	v1 := router.Group("/api/v1")
	allow := func(c *gin.Context) { c.Next() }

	RegisterAdminIdentityRoutes(
		v1,
		&handler.Handlers{
			User:    &handler.UserHandler{},
			Totp:    &handler.TotpHandler{},
			Passkey: &handler.PasskeyHandler{},
		},
		middleware.AdminAuthMiddleware(allow),
		middleware.AuditLogMiddleware(allow),
		nil,
		nil,
	)

	registered := make(map[string]struct{})
	for _, route := range router.Routes() {
		registered[route.Method+" "+route.Path] = struct{}{}
	}

	required := []string{
		"GET /api/v1/user/profile",
		"PUT /api/v1/user/password",
		"GET /api/v1/user/totp/status",
		"POST /api/v1/user/totp/step-up",
	}
	for _, route := range required {
		if _, exists := registered[route]; !exists {
			t.Errorf("administrator identity route is missing: %s", route)
		}
	}

	forbidden := []string{
		"GET /api/v1/keys",
		"GET /api/v1/groups/available",
		"GET /api/v1/usage",
		"POST /api/v1/redeem",
		"GET /api/v1/subscriptions",
		"GET /api/v1/user/aff",
	}
	for _, route := range forbidden {
		if _, exists := registered[route]; exists {
			t.Errorf("customer self-service route is registered: %s", route)
		}
	}
}

func TestAdminRoutesExcludeCustomerAndCommerceManagement(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	allow := func(c *gin.Context) { c.Next() }

	RegisterAdminRoutes(
		router.Group("/api/v1"),
		&handler.Handlers{Admin: &handler.AdminHandlers{}},
		middleware.AdminAuthMiddleware(allow),
		middleware.AuditLogMiddleware(allow),
		middleware.StepUpAuthMiddleware(allow),
		nil,
		nil,
	)

	paths := make([]string, 0, len(router.Routes()))
	for _, route := range router.Routes() {
		paths = append(paths, route.Path)
	}

	requiredPrefixes := []string{
		"/api/v1/admin/groups",
		"/api/v1/admin/accounts",
		"/api/v1/admin/proxies",
	}
	for _, prefix := range requiredPrefixes {
		if !containsRoutePrefix(paths, prefix) {
			t.Errorf("technical management route is missing: %s", prefix)
		}
	}

	forbiddenPrefixes := []string{
		"/api/v1/admin/users",
		"/api/v1/admin/announcements",
		"/api/v1/admin/redeem-codes",
		"/api/v1/admin/promo-codes",
		"/api/v1/admin/subscriptions",
		"/api/v1/admin/user-attributes",
		"/api/v1/admin/api-keys",
		"/api/v1/admin/risk-control",
		"/api/v1/admin/payment",
		"/api/v1/admin/affiliates",
	}
	for _, prefix := range forbiddenPrefixes {
		if containsRoutePrefix(paths, prefix) {
			t.Errorf("customer or commerce route is registered: %s", prefix)
		}
	}
}

func containsRoutePrefix(paths []string, prefix string) bool {
	for _, path := range paths {
		if path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

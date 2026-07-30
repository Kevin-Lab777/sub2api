package routes

import (
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

package handler

import (
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/service"
	"github.com/stretchr/testify/require"
)

func TestEnsureNextAPIAdminLogin(t *testing.T) {
	require.NoError(t, ensureNextAPIAdminLogin(&service.User{Role: service.RoleAdmin, Status: service.StatusActive}))
	require.Error(t, ensureNextAPIAdminLogin(&service.User{Role: service.RoleUser}))
	require.Error(t, ensureNextAPIAdminLogin(nil))
}

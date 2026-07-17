package service

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsReservedEmail_SyntheticDomains(t *testing.T) {
	require.True(t, isReservedEmail("linuxdo-123@linuxdo-connect.invalid"))
	require.True(t, isReservedEmail("oidc-123@oidc-connect.invalid"))
	require.True(t, isReservedEmail("wechat-123@wechat-connect.invalid"))
	require.True(t, isReservedEmail("dingtalk-123@dingtalk-connect.invalid"))
	require.True(t, isReservedEmail("DINGTALK-456@DINGTALK-CONNECT.INVALID"))
	require.False(t, isReservedEmail("real@dingtalk.com"))
}

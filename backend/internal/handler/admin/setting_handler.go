package admin

import "github.com/Wei-Shaw/sub2api/internal/service"

// SettingHandler exposes only technical gateway settings used by Next API.
type SettingHandler struct {
	settingService *service.SettingService
}

func NewSettingHandler(settingService *service.SettingService) *SettingHandler {
	return &SettingHandler{settingService: settingService}
}

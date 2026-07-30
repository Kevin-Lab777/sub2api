package service

import (
	"context"
	"errors"
)

// OpsCapabilities contains the administrator-console switches owned by Ops.
// It is intentionally separate from the former SaaS-wide settings payload.
type OpsCapabilities struct {
	MonitoringEnabled         bool         `json:"monitoring_enabled"`
	RealtimeMonitoringEnabled bool         `json:"realtime_monitoring_enabled"`
	QueryModeDefault          OpsQueryMode `json:"query_mode_default"`
}

func (s *OpsService) GetCapabilities(ctx context.Context) (*OpsCapabilities, error) {
	capabilities := &OpsCapabilities{
		MonitoringEnabled: s.IsMonitoringEnabled(ctx),
		QueryModeDefault:  OpsQueryModeAuto,
	}
	if !capabilities.MonitoringEnabled {
		return capabilities, nil
	}
	if s.settingRepo == nil {
		capabilities.RealtimeMonitoringEnabled = true
		return capabilities, nil
	}

	realtimeValue, err := s.settingRepo.GetValue(ctx, SettingKeyOpsRealtimeMonitoringEnabled)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return nil, err
	}
	capabilities.RealtimeMonitoringEnabled = err != nil || parseOpsMonitoringEnabled(realtimeValue)

	queryModeValue, err := s.settingRepo.GetValue(ctx, SettingKeyOpsQueryModeDefault)
	if err != nil && !errors.Is(err, ErrSettingNotFound) {
		return nil, err
	}
	if err == nil {
		capabilities.QueryModeDefault = ParseOpsQueryMode(queryModeValue)
	}
	return capabilities, nil
}

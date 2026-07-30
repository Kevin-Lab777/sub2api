package service

import (
	"context"
	"errors"
	"testing"
)

func TestOpsCapabilitiesReadDedicatedTechnicalSettings(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	repo.values[SettingKeyOpsRealtimeMonitoringEnabled] = "false"
	repo.values[SettingKeyOpsQueryModeDefault] = "preagg"
	svc := &OpsService{settingRepo: repo}

	got, err := svc.GetCapabilities(context.Background())
	if err != nil {
		t.Fatalf("GetCapabilities() error = %v", err)
	}
	if !got.MonitoringEnabled {
		t.Fatal("MonitoringEnabled = false, want true")
	}
	if got.RealtimeMonitoringEnabled {
		t.Fatal("RealtimeMonitoringEnabled = true, want false")
	}
	if got.QueryModeDefault != OpsQueryModePreagg {
		t.Fatalf("QueryModeDefault = %q, want %q", got.QueryModeDefault, OpsQueryModePreagg)
	}
}

func TestOpsCapabilitiesUseDefinedDefaultsForMissingSettings(t *testing.T) {
	svc := &OpsService{settingRepo: newRuntimeSettingRepoStub()}

	got, err := svc.GetCapabilities(context.Background())
	if err != nil {
		t.Fatalf("GetCapabilities() error = %v", err)
	}
	if !got.RealtimeMonitoringEnabled {
		t.Fatal("RealtimeMonitoringEnabled = false, want default true")
	}
	if got.QueryModeDefault != OpsQueryModeAuto {
		t.Fatalf("QueryModeDefault = %q, want %q", got.QueryModeDefault, OpsQueryModeAuto)
	}
}

func TestOpsCapabilitiesPropagateRepositoryFailure(t *testing.T) {
	wantErr := errors.New("settings unavailable")
	repo := newRuntimeSettingRepoStub()
	repo.getValueFn = func(string) (string, error) { return "", wantErr }
	svc := &OpsService{settingRepo: repo}

	_, err := svc.GetCapabilities(context.Background())
	if !errors.Is(err, wantErr) {
		t.Fatalf("GetCapabilities() error = %v, want %v", err, wantErr)
	}
}

func TestOpsCapabilitiesDoNotReadSoftSettingsWhenMonitoringDisabled(t *testing.T) {
	repo := newRuntimeSettingRepoStub()
	svc := &OpsService{settingRepo: repo}
	svc.runtimeSettings.Store(&opsRuntimeSettingsSnapshot{monitoringEnabled: false})

	got, err := svc.GetCapabilities(context.Background())
	if err != nil {
		t.Fatalf("GetCapabilities() error = %v", err)
	}
	if got.MonitoringEnabled || got.RealtimeMonitoringEnabled {
		t.Fatalf("capabilities = %+v, want monitoring disabled", got)
	}
	if repo.getValueCalls != 0 {
		t.Fatalf("GetValue calls = %d, want 0", repo.getValueCalls)
	}
}

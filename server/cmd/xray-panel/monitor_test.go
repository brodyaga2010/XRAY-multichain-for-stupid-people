package main

import (
	"reflect"
	"testing"
	"time"

	"xraypanel/internal/config"
)

func TestCertDomains(t *testing.T) {
	tests := []struct {
		name      string
		vpnHost   string
		publicURL string
		want      []string
	}{
		{"домен VPN и кабинет", "vpn.example.com", "https://panel.example.com", []string{"vpn.example.com", "panel.example.com"}},
		{"один и тот же домен", "example.com", "https://example.com:443/", []string{"example.com"}},
		{"IP и http — проверять нечего", "203.0.113.1", "http://localhost:5173", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.XrayPublicHost, cfg.PublicBaseURL = tt.vpnHost, tt.publicURL
			if got := certDomains(&cfg); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMonitorDeps(t *testing.T) {
	tests := []struct {
		name      string
		mode      string
		probePort int
		wantProbe string
		wantCerts string
		wantXray  bool
	}{
		{"рабочий сервер", "local", 10811, "127.0.0.1:10811", "vpn.example.com:443", true},
		{"служебный вход выключен", "local", 0, "", "vpn.example.com:443", true},
		{"dry_run — только диск и память", "dry_run", 10811, "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.Defaults()
			cfg.XrayMode, cfg.XrayProbePort = tt.mode, tt.probePort
			cfg.XrayPublicHost = "vpn.example.com"
			cfg.DatabaseURL = "sqlite:////var/lib/xray-panel/panel.db"

			d := monitorDeps(&cfg, nil)

			if d.ProbeAddr != tt.wantProbe || d.CertAddr != tt.wantCerts || (d.XrayOnline != nil) != tt.wantXray ||
				d.DataDir != "/var/lib/xray-panel" {
				t.Errorf("deps = %+v", d)
			}
			if tt.wantXray && (d.ChainSlow != 3*time.Second || d.ExitSlow != 300*time.Millisecond) {
				t.Errorf("пороги: %v %v", d.ChainSlow, d.ExitSlow)
			}
		})
	}
}

func TestNewMonitorDisabled(t *testing.T) {
	cfg := config.Defaults()
	cfg.MonitorIntervalSeconds = 0
	if newMonitor(&cfg, nil) != nil {
		t.Error("MONITOR_INTERVAL_SECONDS=0 должен выключать мониторинг")
	}
}

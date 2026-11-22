package main

import (
	"fmt"
	"testing"

	"xraypanel/internal/config"
)

func TestParseArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantCommand string
		wantEnv     string
		wantErr     bool
	}{
		{"по умолчанию serve", nil, "serve", "", false},
		{"команда", []string{"migrate"}, "migrate", "", false},
		{"флаг до команды", []string{"--env-file", "/etc/xray-panel/panel.env", "seed-admin"}, "seed-admin", "/etc/xray-panel/panel.env", false},
		{"флаг через =", []string{"--env-file=/a.env", "bootstrap-xray"}, "bootstrap-xray", "/a.env", false},
		{"флаг после команды", []string{"serve", "-env-file", "/b.env"}, "serve", "/b.env", false},
		{"только флаг — serve", []string{"--env-file", "/c.env"}, "serve", "/c.env", false},
		{"version", []string{"version"}, "version", "", false},
		{"нет значения флага", []string{"--env-file"}, "", "", true},
		{"неизвестный флаг", []string{"--nope"}, "", "", true},
		{"лишний аргумент", []string{"serve", "extra"}, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			command, env, err := parseArgs(tt.args)
			if (err != nil) != tt.wantErr || command != tt.wantCommand || env != tt.wantEnv {
				t.Errorf("parseArgs = %q, %q, %v", command, env, err)
			}
		})
	}
}

func TestBackupLabel(t *testing.T) {
	tests := []struct{ host, want string }{
		{"kemsekov.ru", "kemsekov"},
		{"vpn.example.com", "vpn"},
		{"localhost", "localhost"},
		{"203.0.113.10", ""},
		{"", ""},
	}
	for _, tt := range tests {
		if got := backupLabel(tt.host); got != tt.want {
			t.Errorf("backupLabel(%q) = %q, want %q", tt.host, got, tt.want)
		}
	}
}

func TestNewMailer(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"SMTP задан", map[string]string{"SMTP_HOST": "smtp.example.com", "SMTP_USERNAME": "noreply@example.com"}, "*mail.SMTP"},
		{"стенд без SMTP — в журнал", map[string]string{"XRAY_MODE": "dry_run"}, "mail.Log"},
		{"сервер без SMTP — выключено", map[string]string{"XRAY_MODE": "local"}, "<nil>"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := map[string]string{"JWT_SECRET": "test-secret-that-is-long-enough", "XRAY_MODE": "dry_run"}
			for k, v := range tt.env {
				env[k] = v
			}
			cfg, err := config.Load(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
			if err != nil {
				t.Fatal(err)
			}
			m, err := newMailer(cfg)
			if got := fmt.Sprintf("%T", m); err != nil || got != tt.want && !(tt.want == "<nil>" && m == nil) {
				t.Errorf("got %s, err %v", got, err)
			}
		})
	}
}

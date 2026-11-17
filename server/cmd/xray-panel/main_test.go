package main

import "testing"

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

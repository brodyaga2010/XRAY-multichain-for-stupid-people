package xray

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func probeInbound(cfg Config) map[string]any {
	for _, item := range asList(cfg["inbounds"]) {
		if inbound := asObj(item); asStr(inbound["tag"]) == ProbeInboundTag {
			return inbound
		}
	}
	return nil
}

func TestEnsureProbeInbound(t *testing.T) {
	tests := []struct {
		name        string
		mutate      func(cfg Config)
		port        int
		wantChanged bool
		wantPort    int
		wantInRule  bool
	}{
		{"добавляется и попадает в правило to-exit", nil, 10811, true, 10811, true},
		{"порт 0 — выключено", nil, 0, false, 0, false},
		{"повторный вызов ничего не меняет", func(cfg Config) { EnsureProbeInbound(cfg, 10811) }, 10811, false, 10811, true},
		{"после разбора JSON порт float64 — не считается изменением", func(cfg Config) {
			EnsureProbeInbound(cfg, 10811)
			raw, _ := MarshalConfig(cfg)
			parsed, _ := ParseConfig(raw)
			for k := range cfg {
				delete(cfg, k)
			}
			for k, v := range parsed {
				cfg[k] = v
			}
		}, 10811, false, 10811, true},
		{"смена порта", func(cfg Config) { EnsureProbeInbound(cfg, 10811) }, 10900, true, 10900, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := legacyRelayConfig(t)
			if tt.mutate != nil {
				tt.mutate(cfg)
			}

			changed := EnsureProbeInbound(cfg, tt.port)

			if changed != tt.wantChanged {
				t.Errorf("changed = %v, want %v", changed, tt.wantChanged)
			}
			inbound := probeInbound(cfg)
			if tt.wantPort == 0 {
				if inbound != nil {
					t.Errorf("служебного входа быть не должно: %v", inbound)
				}
				return
			}
			port, _ := toInt(inbound["port"])
			if port != tt.wantPort || inbound["listen"] != "127.0.0.1" || inbound["protocol"] != "socks" {
				t.Errorf("inbound = %v", inbound)
			}
			rule := asObj(asList(asObj(cfg["routing"])["rules"])[0])
			inRule := false
			for _, tag := range asList(rule["inboundTag"]) {
				inRule = inRule || tag == ProbeInboundTag
			}
			if inRule != tt.wantInRule {
				t.Errorf("inboundTag = %v", rule["inboundTag"])
			}
			if len(UserInbounds(cfg)) != 2 {
				t.Error("служебный вход не должен считаться пользовательским")
			}
		})
	}
}

func TestExitServer(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		wantHost string
		wantPort int
		wantOK   bool
	}{
		{"цепочка", `{"outbounds":[{"tag":"to-exit","settings":{"vnext":[{"address":"212.43.155.144","port":8443}]}}]}`, "212.43.155.144", 8443, true},
		{"одиночный сервер", `{"outbounds":[{"tag":"direct","protocol":"freedom"}]}`, "", 0, false},
		{"нет vnext", `{"outbounds":[{"tag":"to-exit","settings":{}}]}`, "", 0, false},
		{"нет порта", `{"outbounds":[{"tag":"to-exit","settings":{"vnext":[{"address":"x"}]}}]}`, "x", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseConfig([]byte(tt.raw))
			if err != nil {
				t.Fatal(err)
			}
			host, port, ok := ExitServer(cfg)
			if !reflect.DeepEqual([]any{host, port, ok}, []any{tt.wantHost, tt.wantPort, tt.wantOK}) {
				t.Errorf("got %q %d %v", host, port, ok)
			}
		})
	}
}

func TestEnsureExitHealthCheck(t *testing.T) {
	const grpcExit = `{"outbounds":[{"tag":"to-exit","protocol":"vless","streamSettings":{"network":"grpc","grpcSettings":{"serviceName":"svc"}}}]}`
	tests := []struct {
		name        string
		raw         string
		wantChanged bool
		wantGRPC    map[string]any // nil — grpcSettings не должно быть
	}{
		{"gRPC до выхода — включаем проверку", grpcExit, true,
			map[string]any{"serviceName": "svc", "idle_timeout": exitIdleTimeout, "health_check_timeout": exitHealthTimeout}},
		{"без grpcSettings — создаём", `{"outbounds":[{"tag":"to-exit","streamSettings":{"network":"grpc"}}]}`, true,
			map[string]any{"idle_timeout": exitIdleTimeout, "health_check_timeout": exitHealthTimeout}},
		{"заданное вручную не трогаем", `{"outbounds":[{"tag":"to-exit","streamSettings":{"network":"grpc","grpcSettings":{"idle_timeout":60,"health_check_timeout":20}}}]}`, false,
			map[string]any{"idle_timeout": 60, "health_check_timeout": 20}},
		{"выход по TCP — не трогаем", `{"outbounds":[{"tag":"to-exit","streamSettings":{"network":"tcp"}}]}`, false, nil},
		{"одиночный сервер", `{"outbounds":[{"tag":"direct","protocol":"freedom"}]}`, false, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseConfig([]byte(tt.raw))
			if err != nil {
				t.Fatal(err)
			}
			if got := EnsureExitHealthCheck(cfg); got != tt.wantChanged {
				t.Errorf("changed = %v, want %v", got, tt.wantChanged)
			}
			stream := asObj(asObj(asList(cfg["outbounds"])[0])["streamSettings"])
			grpc := asObj(stream["grpcSettings"])
			if tt.wantGRPC == nil {
				if grpc != nil {
					t.Errorf("grpcSettings = %v, ждём без них", grpc)
				}
				return
			}
			got, _ := json.Marshal(grpc)
			want, _ := json.Marshal(tt.wantGRPC)
			if string(got) != string(want) {
				t.Errorf("grpcSettings = %v, want %v", grpc, tt.wantGRPC)
			}
			if EnsureExitHealthCheck(cfg) {
				t.Error("повторный вызов снова меняет конфиг")
			}
		})
	}
}

const grpcExitOutbound = `{"tag":"to-exit","protocol":"vless",
	"settings":{"vnext":[{"address":"203.0.113.7","port":8443,"users":[{"id":"u1","encryption":"none"}]}]},
	"streamSettings":{"network":"grpc","security":"reality","realitySettings":{"serverName":"www.apple.com"},
		"grpcSettings":{"serviceName":"svc","idle_timeout":15}}}`

func TestEnsureExitTCP(t *testing.T) {
	tests := []struct {
		name        string
		raw         string
		port        int
		wantChanged bool
		wantExit    string // JSON outbound to-exit после изменения; "" — не проверяем
	}{
		{"gRPC → TCP + Vision", `{"outbounds":[` + grpcExitOutbound + `]}`, 443, true,
			`{"protocol":"vless","settings":{"vnext":[{"address":"203.0.113.7","port":443,"users":[{"encryption":"none","flow":"xtls-rprx-vision","id":"u1"}]}]},` +
				`"streamSettings":{"network":"tcp","realitySettings":{"serverName":"www.apple.com"},"security":"reality"},"tag":"to-exit"}`},
		{"порт 0 — не трогаем", `{"outbounds":[` + grpcExitOutbound + `]}`, 0, false, ""},
		{"уже TCP", `{"outbounds":[{"tag":"to-exit","streamSettings":{"network":"tcp"}}]}`, 443, false, ""},
		{"одиночный сервер", `{"outbounds":[{"tag":"direct","protocol":"freedom"}]}`, 443, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseConfig([]byte(tt.raw))
			if err != nil {
				t.Fatal(err)
			}
			if got := EnsureExitTCP(cfg, tt.port); got != tt.wantChanged {
				t.Fatalf("changed = %v, want %v", got, tt.wantChanged)
			}
			if tt.wantExit != "" {
				got, _ := json.Marshal(asList(cfg["outbounds"])[0])
				if string(got) != tt.wantExit {
					t.Errorf("to-exit = %s\nwant      %s", got, tt.wantExit)
				}
			}
			if EnsureExitTCP(cfg, tt.port) {
				t.Error("повторный вызов снова меняет конфиг")
			}
		})
	}
}

func TestBootstrapSwitchesExitToTCP(t *testing.T) {
	tests := []struct {
		name        string
		env         map[string]string
		dialErr     error
		wantNetwork string
		wantPort    int
		wantDialed  string
	}{
		{"TCP-порт выхода открыт — переходим", nil, nil, "tcp", 443, "203.0.113.7:443"},
		{"другой порт из настроек", map[string]string{"XRAY_EXIT_TCP_PORT": "9443"}, nil, "tcp", 9443, "203.0.113.7:9443"},
		{"порт закрыт — остаёмся на gRPC", nil, errors.New("connection refused"), "grpc", 8443, "203.0.113.7:443"},
		{"переход выключен", map[string]string{"XRAY_EXIT_TCP_PORT": "0"}, nil, "grpc", 8443, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := testConfig(t, tt.env)
			base := DryRunConfig()
			var exit map[string]any
			if err := json.Unmarshal([]byte(grpcExitOutbound), &exit); err != nil {
				t.Fatal(err)
			}
			base["outbounds"] = []any{exit}
			raw, err := MarshalConfig(base)
			if err != nil {
				t.Fatal(err)
			}
			runner := NewMemoryRunner(map[string]string{cfg.XrayConfigPath: string(raw)})
			m := NewManager(cfg, runner)
			dialed := ""
			m.dial = func(addr string) error { dialed = addr; return tt.dialErr }

			if _, err := m.Bootstrap(); err != nil {
				t.Fatal(err)
			}

			saved, err := ParseConfig([]byte(runner.Files[cfg.XrayConfigPath]))
			if err != nil {
				t.Fatal(err)
			}
			_, stream := exitOutbound(saved)
			_, port, _ := ExitServer(saved)
			if asStr(stream["network"]) != tt.wantNetwork || port != tt.wantPort || dialed != tt.wantDialed {
				t.Errorf("network=%v port=%d dialed=%q", stream["network"], port, dialed)
			}
			if tt.wantNetwork == "grpc" && asObj(stream["grpcSettings"])["health_check_timeout"] == nil {
				t.Error("оставшись на gRPC, связь должна проверяться (health_check_timeout)")
			}
		})
	}
}

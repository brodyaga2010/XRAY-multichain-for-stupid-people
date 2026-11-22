package xray

import (
	"net/url"
	"reflect"
	"strings"
	"testing"
)

const (
	uuidA = "11111111-1111-1111-1111-111111111111"
	uuidB = "22222222-2222-2222-2222-222222222222"
)

func mustConfig(t *testing.T, raw string) Config {
	t.Helper()
	cfg, err := ParseConfig([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

// legacyRelayConfig — конфиг, как его создаёт generate_config.sh: TCP + gRPC, разные email у одного UUID.
func legacyRelayConfig(t *testing.T) Config {
	t.Helper()
	reality := `{"dest":"127.0.0.1:8444","serverNames":["vpn.example.com",""],"privateKey":"priv","shortIds":["abcd"]}`
	return mustConfig(t, `{
	  "inbounds": [
	    {"tag":"user-inbound","port":443,"protocol":"vless",
	     "settings":{"clients":[{"id":"`+uuidA+`","flow":"xtls-rprx-vision","email":"first"}]},
	     "streamSettings":{"network":"tcp","security":"reality","realitySettings":`+reality+`}},
	    {"tag":"user-inbound-grpc","port":8443,"protocol":"vless",
	     "settings":{"clients":[{"id":"`+uuidA+`","flow":"","email":"first-grpc"}]},
	     "streamSettings":{"network":"grpc","security":"reality","realitySettings":`+reality+`,"grpcSettings":{"serviceName":"svc"}}}
	  ],
	  "outbounds": [{"protocol":"vless","tag":"to-exit"},{"protocol":"freedom","tag":"direct"}],
	  "routing": {"rules":[{"type":"field","inboundTag":["user-inbound","user-inbound-grpc"],"outboundTag":"to-exit"}]}
	}`)
}

// clientsByTag — клиенты каждого inbound в виде простых структур для сравнения.
// clientsByTag — клиенты пользовательских inbound (служебный вход мониторинга не входит).
func clientsByTag(cfg Config) map[string][]map[string]any {
	out := map[string][]map[string]any{}
	for _, inbound := range UserInbounds(cfg) {
		var clients []map[string]any
		for _, c := range asList(asObj(inbound["settings"])["clients"]) {
			clients = append(clients, asObj(c))
		}
		out[asStr(inbound["tag"])] = clients
	}
	return out
}

func inboundTags(cfg Config) []string {
	var tags []string
	for _, item := range asList(cfg["inbounds"]) {
		tags = append(tags, asStr(asObj(item)["tag"]))
	}
	return tags
}

func TestEnsurePanelFeaturesAddsAPIStatsXHTTPAndRouting(t *testing.T) {
	cfg := legacyRelayConfig(t)

	changed, err := EnsurePanelFeatures(cfg, "127.0.0.1:10085", 2053)

	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if got := asObj(cfg["api"])["listen"]; got != "127.0.0.1:10085" {
		t.Errorf("api.listen = %v", got)
	}
	level0 := asObj(asObj(asObj(cfg["policy"])["levels"])["0"])
	if level0["statsUserOnline"] != true {
		t.Errorf("statsUserOnline не включён: %v", level0)
	}
	var xhttp map[string]any
	for _, item := range asList(cfg["inbounds"]) {
		if asStr(asObj(item)["tag"]) == "user-inbound-xhttp" {
			xhttp = asObj(item)
		}
	}
	if xhttp == nil {
		t.Fatal("нет inbound user-inbound-xhttp")
	}
	if xhttp["port"] != 2053 {
		t.Errorf("port = %v", xhttp["port"])
	}
	if pk := asObj(asObj(xhttp["streamSettings"])["realitySettings"])["privateKey"]; pk != "priv" {
		t.Errorf("privateKey = %v", pk)
	}
	rule := asObj(asList(asObj(cfg["routing"])["rules"])[0])
	if !reflect.DeepEqual(rule["inboundTag"], []any{"user-inbound", "user-inbound-grpc", "user-inbound-xhttp"}) {
		t.Errorf("inboundTag = %v", rule["inboundTag"])
	}
	// один UUID во всех inbound, email = UUID, flow по виду подключения
	want := map[string][]map[string]any{
		"user-inbound":       {{"id": uuidA, "email": uuidA, "flow": "xtls-rprx-vision"}},
		"user-inbound-grpc":  {{"id": uuidA, "email": uuidA, "flow": ""}},
		"user-inbound-xhttp": {{"id": uuidA, "email": uuidA, "flow": ""}},
	}
	if got := clientsByTag(cfg); !reflect.DeepEqual(got, want) {
		t.Errorf("clients = %v, want %v", got, want)
	}
}

func TestEnsurePanelFeaturesIsIdempotent(t *testing.T) {
	cfg := legacyRelayConfig(t)
	if _, err := EnsurePanelFeatures(cfg, "127.0.0.1:10085", 2053); err != nil {
		t.Fatal(err)
	}
	snapshot := deepCopy(cfg)

	changed, err := EnsurePanelFeatures(cfg, "127.0.0.1:10085", 2053)

	if err != nil || changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if !reflect.DeepEqual(cfg, snapshot) {
		t.Error("конфиг изменился при повторном вызове")
	}
}

func TestEnsurePanelFeaturesWithoutXHTTP(t *testing.T) {
	cfg := legacyRelayConfig(t)

	if _, err := EnsurePanelFeatures(cfg, "127.0.0.1:10085", 0); err != nil {
		t.Fatal(err)
	}

	if got := inboundTags(cfg); !reflect.DeepEqual(got, []string{"user-inbound", "user-inbound-grpc"}) {
		t.Errorf("inbounds = %v", got)
	}
}

func TestEnsurePanelFeaturesRequiresPrimaryInbound(t *testing.T) {
	cfg := legacyRelayConfig(t)
	cfg["inbounds"] = asList(cfg["inbounds"])[1:]

	_, err := EnsurePanelFeatures(cfg, "127.0.0.1:10085", 2053)

	var cerr *ConfigError
	if !asConfigError(err, &cerr) {
		t.Fatalf("ожидалась ConfigError, получено %v", err)
	}
}

func asConfigError(err error, target **ConfigError) bool {
	ce, ok := err.(*ConfigError)
	*target = ce
	return ok
}

func TestAddAndRemoveClientInAllInbounds(t *testing.T) {
	cfg := DryRunConfig()

	added, _ := AddClient(cfg, uuidB)
	addedAgain, _ := AddClient(cfg, uuidB)
	removed := RemoveClient(cfg, uuidB)
	removedAgain := RemoveClient(cfg, uuidB)

	tags := []string{"user-inbound", "user-inbound-grpc", "user-inbound-xhttp"}
	if !reflect.DeepEqual(added, tags) {
		t.Errorf("added = %v", added)
	}
	if len(addedAgain) != 0 {
		t.Errorf("addedAgain = %v", addedAgain)
	}
	var wantRemoved []Removed
	for _, tag := range tags {
		wantRemoved = append(wantRemoved, Removed{Tag: tag, Email: uuidB})
	}
	if !reflect.DeepEqual(removed, wantRemoved) {
		t.Errorf("removed = %v", removed)
	}
	if len(removedAgain) != 0 {
		t.Errorf("removedAgain = %v", removedAgain)
	}
	for tag, clients := range clientsByTag(cfg) {
		if len(clients) != 0 {
			t.Errorf("%s: остались клиенты %v", tag, clients)
		}
	}
}

func TestAddClientWithoutUserInboundsFails(t *testing.T) {
	_, err := AddClient(Config{"inbounds": []any{}}, uuidA)
	if _, ok := err.(*ConfigError); !ok {
		t.Fatalf("ожидалась ConfigError, получено %v", err)
	}
}

func TestListClientIDsUniqueInOrder(t *testing.T) {
	cfg := DryRunConfig()
	_, _ = AddClient(cfg, uuidB)
	_, _ = AddClient(cfg, uuidA)

	if got := ListClientIDs(cfg); !reflect.DeepEqual(got, []string{uuidB, uuidA}) {
		t.Errorf("ids = %v", got)
	}
}

func TestServerProfileValidation(t *testing.T) {
	tests := []struct {
		name        string
		serverNames []any
		shortIDs    []any
		privateKey  string
		wantErr     string
	}{
		{"ok, пустое имя первым", []any{"", "vpn.example.com"}, []any{"abcd"}, "priv", ""},
		{"нет serverNames", []any{""}, []any{"abcd"}, "priv", "serverNames"},
		{"нет shortIds", []any{"vpn.example.com"}, []any{}, "priv", "shortIds"},
		{"нет privateKey", []any{"vpn.example.com"}, []any{"abcd"}, "", "privateKey"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := legacyRelayConfig(t)
			reality := asObj(asObj(asObj(asList(cfg["inbounds"])[0])["streamSettings"])["realitySettings"])
			reality["serverNames"], reality["shortIds"], reality["privateKey"] = tt.serverNames, tt.shortIDs, tt.privateKey

			profile, err := BuildServerProfile(cfg)

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ожидалась ошибка с %q, получено %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if profile.SNI != "vpn.example.com" {
				t.Errorf("sni = %q", profile.SNI)
			}
			var got [][2]any
			for _, item := range profile.Transports {
				got = append(got, [2]any{item.Transport.Key, item.Port})
			}
			if want := [][2]any{{"tcp", 443}, {"grpc", 8443}}; !reflect.DeepEqual(got, want) {
				t.Errorf("transports = %v", got)
			}
		})
	}
}

func TestBuildLinksForAllTransportsWithEncodedName(t *testing.T) {
	profile, err := BuildServerProfile(DryRunConfig())
	if err != nil {
		t.Fatal(err)
	}

	links := BuildLinks(profile, "vpn.example.com", "PUB", uuidA, "Мой телефон")

	var keys []string
	var parsed []*url.URL
	for _, l := range links {
		keys = append(keys, l.Transport)
		u, err := url.Parse(l.URI)
		if err != nil {
			t.Fatal(err)
		}
		parsed = append(parsed, u)
	}
	if !reflect.DeepEqual(keys, []string{"tcp", "grpc", "xhttp"}) {
		t.Fatalf("transports = %v", keys)
	}
	for i, wantPort := range []string{"443", "8443", "2053"} {
		u := parsed[i]
		if u.Port() != wantPort || u.User.Username() != uuidA || u.Hostname() != "vpn.example.com" {
			t.Errorf("link %d: %s", i, links[i].URI)
		}
	}
	if parsed[0].Fragment != "Мой телефон TCP" {
		t.Errorf("fragment = %q", parsed[0].Fragment)
	}
	if strings.Contains(links[0].URI, " ") || !strings.Contains(links[0].URI, "#%D0%9C%D0%BE%D0%B9%20%D1%82") {
		t.Errorf("фрагмент должен кодироваться как urllib quote: %s", links[0].URI)
	}
	tcp, grpc, xhttp := parsed[0].Query(), parsed[1].Query(), parsed[2].Query()
	if tcp.Get("flow") != "xtls-rprx-vision" || tcp.Get("type") != "tcp" {
		t.Errorf("tcp: %v", tcp)
	}
	if tcp.Get("sni") != "vpn.example.com" || tcp.Get("pbk") != "PUB" || tcp.Get("sid") != "dryrun01" {
		t.Errorf("tcp: %v", tcp)
	}
	if grpc.Has("flow") || grpc.Get("serviceName") != "xray_grpc_service" || grpc.Get("mode") != "gun" {
		t.Errorf("grpc: %v", grpc)
	}
	if xhttp.Get("type") != "xhttp" || xhttp.Get("path") != "/download/updates" {
		t.Errorf("xhttp: %v", xhttp)
	}
	// порядок и кодирование query-параметров как в Python
	wantQuery := "encryption=none&flow=xtls-rprx-vision&security=reality&sni=vpn.example.com&fp=chrome&pbk=PUB&sid=dryrun01&type=tcp"
	if parsed[0].RawQuery != wantQuery {
		t.Errorf("query = %s", parsed[0].RawQuery)
	}
	wantXHTTP := "encryption=none&security=reality&sni=vpn.example.com&fp=chrome&pbk=PUB&sid=dryrun01&type=xhttp&path=%2Fdownload%2Fupdates&mode=auto"
	if parsed[2].RawQuery != wantXHTTP {
		t.Errorf("xhttp query = %s", parsed[2].RawQuery)
	}
}

func TestParsePublicKey(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		want    string
		wantErr bool
	}{
		{"новый формат", "PrivateKey: a\nPassword (PublicKey): NEWFORMAT\nHash32: z\n", "NEWFORMAT", false},
		{"старый формат", "Private key: a\nPublic key: OLDFORMAT\n", "OLDFORMAT", false},
		{"нет публичного ключа", "PrivateKey: a\n", "", true},
		{"средний формат", "PrivateKey: a\nPublicKey: MIDFORMAT\n", "MIDFORMAT", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePublicKey(tt.output)
			if (err != nil) != tt.wantErr || got != tt.want {
				t.Errorf("ParsePublicKey = %q, %v", got, err)
			}
		})
	}
}

func TestShellSplit(t *testing.T) {
	tests := []struct {
		in      string
		want    []string
		wantErr bool
	}{
		{"systemctl restart xray", []string{"systemctl", "restart", "xray"}, false},
		{`sh -c "systemctl restart xray"`, []string{"sh", "-c", "systemctl restart xray"}, false},
		{`echo 'a b'  c\ d`, []string{"echo", "a b", "c d"}, false},
		{`echo ""`, []string{"echo", ""}, false},
		{"", nil, false},
		{`echo "oops`, nil, true},
	}
	for _, tt := range tests {
		got, err := ShellSplit(tt.in)
		if (err != nil) != tt.wantErr || !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ShellSplit(%q) = %q, %v", tt.in, got, err)
		}
	}
}

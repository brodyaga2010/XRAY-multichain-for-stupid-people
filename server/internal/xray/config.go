// Package xray управляет пользователями Xray на входном (relay) сервере.
//
// Файл config.go — чистые функции над config.json. Ключ = один UUID, который присутствует
// во всех пользовательских inbound (TCP, gRPC, XHTTP); у клиента email = UUID, по нему
// считаются трафик и онлайн.
package xray

import (
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"strconv"
	"strings"
)

// Config — разобранный config.json (числа хранятся как json.Number).
type Config = map[string]any

// Transport — вид подключения.
type Transport struct {
	Key     string
	Tag     string
	Network string
	Flow    string
	Label   string
}

// Transports — виды подключения в порядке вывода.
var Transports = []Transport{
	{"tcp", "user-inbound", "tcp", "xtls-rprx-vision", "TCP"},
	{"grpc", "user-inbound-grpc", "grpc", "", "gRPC"},
	{"xhttp", "user-inbound-xhttp", "xhttp", "", "XHTTP"},
}

const (
	PrimaryTag         = "user-inbound"
	DefaultGRPCService = "xray_grpc_service"
	DefaultXHTTPPath   = "/download/updates"
	ExitOutboundTag    = "to-exit"
	ProbeInboundTag    = "panel-probe"
	xhttpTag           = "user-inbound-xhttp"
)

func transportByTag(tag string) Transport {
	for _, t := range Transports {
		if t.Tag == tag {
			return t
		}
	}
	return Transport{}
}

// ConfigError — конфиг Xray непригоден для панели.
type ConfigError struct{ Msg string }

func (e *ConfigError) Error() string { return e.Msg }

func configErr(format string, args ...any) error {
	return &ConfigError{Msg: fmt.Sprintf(format, args...)}
}

// TransportProfile — параметры одного вида подключения на сервере.
type TransportProfile struct {
	Transport   Transport
	Port        int
	GRPCService *string
	XHTTPPath   *string
}

// ServerProfile — параметры Reality и транспортов сервера.
type ServerProfile struct {
	SNI        string
	ShortID    string
	PrivateKey string
	Transports []TransportProfile
}

// KeyLink — ссылка подключения для одного вида.
type KeyLink struct {
	Transport string
	Label     string
	URI       string
}

// ---------- вспомогательные функции над динамическим JSON ----------

func asObj(v any) map[string]any {
	m, _ := v.(map[string]any)
	return m
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

func asStr(v any) string {
	s, _ := v.(string)
	return s
}

func toInt(v any) (int, error) {
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i), nil
		}
		f, err := n.Float64()
		return int(f), err
	case string:
		return strconv.Atoi(strings.TrimSpace(n))
	}
	return 0, fmt.Errorf("не число: %v", v)
}

// setDefaultObj — аналог dict.setdefault(key, {}).
func setDefaultObj(m map[string]any, key string) map[string]any {
	if c := asObj(m[key]); c != nil {
		return c
	}
	c := map[string]any{}
	m[key] = c
	return c
}

func deepCopy(v any) any {
	switch t := v.(type) {
	case map[string]any:
		if t == nil {
			return t
		}
		out := make(map[string]any, len(t))
		for k, item := range t {
			out[k] = deepCopy(item)
		}
		return out
	case []any:
		if t == nil {
			return t
		}
		out := make([]any, len(t))
		for i, item := range t {
			out[i] = deepCopy(item)
		}
		return out
	default:
		return v
	}
}

// ParseConfig разбирает JSON конфига.
func ParseConfig(raw []byte) (Config, error) {
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var cfg Config
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("конфиг не объект")
	}
	return cfg, nil
}

// MarshalConfig — JSON с отступом в 2 пробела и переводом строки в конце (как json.dumps(indent=2)).
func MarshalConfig(cfg Config) ([]byte, error) {
	var sb strings.Builder
	enc := json.NewEncoder(&sb)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(cfg); err != nil {
		return nil, err
	}
	return []byte(sb.String()), nil
}

// ---------- работа с inbound и клиентами ----------

// UserInbounds — пользовательские inbound в порядке Transports (отсутствующие пропускаются).
func UserInbounds(cfg Config) []map[string]any {
	byTag := map[string]map[string]any{}
	for _, item := range asList(cfg["inbounds"]) {
		if inbound := asObj(item); inbound != nil {
			byTag[asStr(inbound["tag"])] = inbound
		}
	}
	var out []map[string]any
	for _, t := range Transports {
		if inbound, ok := byTag[t.Tag]; ok {
			out = append(out, inbound)
		}
	}
	return out
}

func clients(inbound map[string]any) []any {
	settings := setDefaultObj(inbound, "settings")
	list, ok := settings["clients"].([]any)
	if !ok {
		list = []any{}
		settings["clients"] = list
	}
	return list
}

func setClients(inbound map[string]any, list []any) {
	setDefaultObj(inbound, "settings")["clients"] = list
}

// ClientEntry — запись клиента для inbound.
func ClientEntry(inbound map[string]any, keyUUID string) map[string]any {
	return map[string]any{"id": keyUUID, "email": keyUUID, "flow": transportByTag(asStr(inbound["tag"])).Flow}
}

// ListClientIDs — все UUID из пользовательских inbound без повторов, в порядке появления.
func ListClientIDs(cfg Config) []string {
	seen := map[string]bool{}
	var ids []string
	for _, inbound := range UserInbounds(cfg) {
		for _, item := range clients(inbound) {
			id := asStr(asObj(item)["id"])
			if id != "" && !seen[id] {
				seen[id] = true
				ids = append(ids, id)
			}
		}
	}
	return ids
}

// AddClient добавляет UUID во все пользовательские inbound и возвращает теги, куда он реально добавлен.
func AddClient(cfg Config, keyUUID string) ([]string, error) {
	inbounds := UserInbounds(cfg)
	if len(inbounds) == 0 {
		return nil, configErr("В конфиге Xray нет пользовательских inbound")
	}
	var added []string
	for _, inbound := range inbounds {
		list := clients(inbound)
		exists := false
		for _, item := range list {
			if asStr(asObj(item)["id"]) == keyUUID {
				exists = true
				break
			}
		}
		if exists {
			continue
		}
		setClients(inbound, append(list, ClientEntry(inbound, keyUUID)))
		added = append(added, asStr(inbound["tag"]))
	}
	return added, nil
}

// Removed — удалённый клиент: тег inbound и его email.
type Removed struct {
	Tag   string
	Email string
}

// RemoveClient удаляет UUID из всех пользовательских inbound.
func RemoveClient(cfg Config, keyUUID string) []Removed {
	var removed []Removed
	for _, inbound := range UserInbounds(cfg) {
		keep := make([]any, 0)
		for _, item := range clients(inbound) {
			client := asObj(item)
			if asStr(client["id"]) == keyUUID {
				removed = append(removed, Removed{Tag: asStr(inbound["tag"]), Email: asStr(client["email"])})
			} else {
				keep = append(keep, item)
			}
		}
		setClients(inbound, keep)
	}
	return removed
}

// NormalizeClients: email = UUID у всех клиентов, правильный flow, каждый UUID во всех inbound.
// Возвращает true, если конфиг изменился (нужен перезапуск Xray).
func NormalizeClients(cfg Config) bool {
	before := deepCopy(cfg)
	ids := ListClientIDs(cfg)
	for _, inbound := range UserInbounds(cfg) {
		list := make([]any, 0, len(ids))
		for _, id := range ids {
			list = append(list, ClientEntry(inbound, id))
		}
		setClients(inbound, list)
	}
	return !reflect.DeepEqual(before, cfg)
}

// EnsurePanelFeatures включает API/статистику Xray и (при xhttpPort > 0) XHTTP inbound. Идемпотентно.
// Возвращает true, если конфиг изменился.
func EnsurePanelFeatures(cfg Config, apiListen string, xhttpPort int) (bool, error) {
	before := deepCopy(cfg)

	cfg["api"] = map[string]any{"tag": "api", "listen": apiListen, "services": []any{"HandlerService", "StatsService"}}
	if asObj(cfg["stats"]) == nil {
		cfg["stats"] = map[string]any{}
	}
	levels := setDefaultObj(setDefaultObj(cfg, "policy"), "levels")
	level0 := setDefaultObj(levels, "0")
	level0["statsUserUplink"] = true
	level0["statsUserDownlink"] = true
	level0["statsUserOnline"] = true

	byTag := map[string]map[string]any{}
	for _, item := range asList(cfg["inbounds"]) {
		if inbound := asObj(item); inbound != nil {
			byTag[asStr(inbound["tag"])] = inbound
		}
	}
	primary, ok := byTag[PrimaryTag]
	if !ok {
		return false, configErr("Не найден inbound %s", PrimaryTag)
	}

	if _, exists := byTag[xhttpTag]; xhttpPort > 0 && !exists {
		reality := asObj(asObj(primary["streamSettings"])["realitySettings"])
		if reality == nil {
			return false, configErr("В inbound %s нет realitySettings", PrimaryTag)
		}
		cfg["inbounds"] = append(asList(cfg["inbounds"]), map[string]any{
			"listen":   "0.0.0.0",
			"port":     xhttpPort,
			"protocol": "vless",
			"tag":      xhttpTag,
			"settings": map[string]any{"clients": []any{}, "decryption": "none"},
			"streamSettings": map[string]any{
				"network":         "xhttp",
				"security":        "reality",
				"realitySettings": deepCopy(reality),
				"xhttpSettings":   map[string]any{"path": DefaultXHTTPPath, "mode": "auto"},
			},
			"sniffing": map[string]any{"enabled": true, "destOverride": []any{"http", "tls", "quic"}},
		})
	}

	var present []string
	for _, inbound := range UserInbounds(cfg) {
		present = append(present, asStr(inbound["tag"]))
	}
	addToExitRule(cfg, present)

	NormalizeClients(cfg)
	return !reflect.DeepEqual(before, cfg), nil
}

// addToExitRule дописывает inbound-теги в правило маршрутизации «→ to-exit», если оно есть
// (на одиночном сервере такого правила нет — трафик уходит в outbound по умолчанию).
func addToExitRule(cfg Config, inboundTags []string) {
	routing := setDefaultObj(cfg, "routing")
	rules, ok := routing["rules"].([]any)
	if !ok {
		rules = []any{}
		routing["rules"] = rules
	}
	for _, item := range rules {
		rule := asObj(item)
		if rule == nil || asStr(rule["outboundTag"]) != ExitOutboundTag {
			continue
		}
		tags, _ := rule["inboundTag"].([]any)
		if tags == nil {
			tags = []any{}
		}
		for _, tag := range inboundTags {
			found := false
			for _, existing := range tags {
				if asStr(existing) == tag {
					found = true
					break
				}
			}
			if !found {
				tags = append(tags, tag)
			}
		}
		rule["inboundTag"] = tags
		return
	}
}

// EnsureProbeInbound добавляет служебный SOCKS-вход на 127.0.0.1:port для проверки цепочки мониторингом.
// Трафик через него маршрутизируется так же, как трафик пользователей. port <= 0 — ничего не делает.
// Возвращает true, если конфиг изменился.
func EnsureProbeInbound(cfg Config, port int) bool {
	if port <= 0 {
		return false
	}
	before := deepCopy(cfg)
	want := map[string]any{
		"tag":      ProbeInboundTag,
		"listen":   "127.0.0.1",
		"port":     port,
		"protocol": "socks",
		"settings": map[string]any{"auth": "noauth", "udp": false},
	}
	inbounds := asList(cfg["inbounds"])
	replaced := false
	for i, item := range inbounds {
		inbound := asObj(item)
		if inbound == nil || asStr(inbound["tag"]) != ProbeInboundTag {
			continue
		}
		current, _ := toInt(inbound["port"])
		if current != port || asStr(inbound["listen"]) != "127.0.0.1" || asStr(inbound["protocol"]) != "socks" {
			inbounds[i] = want
		}
		replaced = true
	}
	if !replaced {
		cfg["inbounds"] = append(inbounds, want)
	}
	addToExitRule(cfg, []string{ProbeInboundTag})
	return !reflect.DeepEqual(before, cfg)
}

// ExitServer — адрес выходного сервера из outbound to-exit; ok=false, если цепочки нет.
func ExitServer(cfg Config) (host string, port int, ok bool) {
	for _, item := range asList(cfg["outbounds"]) {
		outbound := asObj(item)
		if outbound == nil || asStr(outbound["tag"]) != ExitOutboundTag {
			continue
		}
		vnext := asList(asObj(outbound["settings"])["vnext"])
		if len(vnext) == 0 {
			return "", 0, false
		}
		server := asObj(vnext[0])
		port, err := toInt(server["port"])
		host = asStr(server["address"])
		return host, port, err == nil && host != "" && port > 0
	}
	return "", 0, false
}

// BuildServerProfile извлекает из конфига параметры Reality и транспортов.
func BuildServerProfile(cfg Config) (ServerProfile, error) {
	inbounds := UserInbounds(cfg)
	if len(inbounds) == 0 || asStr(inbounds[0]["tag"]) != PrimaryTag {
		return ServerProfile{}, configErr("Не найден inbound %s", PrimaryTag)
	}
	reality := asObj(asObj(inbounds[0]["streamSettings"])["realitySettings"])
	var names, shortIDs []string
	for _, v := range asList(reality["serverNames"]) {
		if s := asStr(v); s != "" {
			names = append(names, s)
		}
	}
	for _, v := range asList(reality["shortIds"]) {
		if s := asStr(v); s != "" {
			shortIDs = append(shortIDs, s)
		}
	}
	if len(names) == 0 {
		return ServerProfile{}, configErr("В realitySettings.serverNames нет непустого имени")
	}
	if len(shortIDs) == 0 {
		return ServerProfile{}, configErr("В realitySettings.shortIds нет значения")
	}
	privateKey := asStr(reality["privateKey"])
	if privateKey == "" {
		return ServerProfile{}, configErr("В realitySettings нет privateKey")
	}

	profile := ServerProfile{SNI: names[0], ShortID: shortIDs[0], PrivateKey: privateKey}
	for _, inbound := range inbounds {
		transport := transportByTag(asStr(inbound["tag"]))
		stream := asObj(inbound["streamSettings"])
		port, err := toInt(inbound["port"])
		if err != nil {
			return ServerProfile{}, configErr("Некорректный port у inbound %s", transport.Tag)
		}
		item := TransportProfile{Transport: transport, Port: port}
		if transport.Network == "grpc" {
			service := DefaultGRPCService
			if v, ok := asObj(stream["grpcSettings"])["serviceName"]; ok {
				service = asStr(v)
			}
			item.GRPCService = &service
		}
		if transport.Network == "xhttp" {
			path := DefaultXHTTPPath
			if v, ok := asObj(stream["xhttpSettings"])["path"]; ok {
				path = asStr(v)
			}
			item.XHTTPPath = &path
		}
		profile.Transports = append(profile.Transports, item)
	}
	return profile, nil
}

// quote — аналог urllib.parse.quote(value, safe="").
func quote(value string) string {
	return strings.ReplaceAll(url.QueryEscape(value), "+", "%20")
}

// BuildLinks собирает ссылки vless:// для всех видов подключения.
func BuildLinks(profile ServerProfile, host, publicKey, keyUUID, name string) []KeyLink {
	links := make([]KeyLink, 0, len(profile.Transports))
	for _, item := range profile.Transports {
		params := [][2]string{{"encryption", "none"}}
		if item.Transport.Flow != "" {
			params = append(params, [2]string{"flow", item.Transport.Flow})
		}
		params = append(params,
			[2]string{"security", "reality"},
			[2]string{"sni", profile.SNI},
			[2]string{"fp", "chrome"},
			[2]string{"pbk", publicKey},
			[2]string{"sid", profile.ShortID},
			[2]string{"type", item.Transport.Network},
		)
		if item.GRPCService != nil {
			params = append(params, [2]string{"serviceName", *item.GRPCService}, [2]string{"mode", "gun"})
		}
		if item.XHTTPPath != nil {
			params = append(params, [2]string{"path", *item.XHTTPPath}, [2]string{"mode", "auto"})
		}
		parts := make([]string, 0, len(params))
		for _, p := range params {
			parts = append(parts, quote(p[0])+"="+quote(p[1]))
		}
		fragment := quote(name + " " + item.Transport.Label)
		uri := fmt.Sprintf("vless://%s@%s:%d?%s#%s", keyUUID, host, item.Port, strings.Join(parts, "&"), fragment)
		links = append(links, KeyLink{Transport: item.Transport.Key, Label: item.Transport.Label, URI: uri})
	}
	return links
}

// ParsePublicKey достаёт публичный ключ из вывода `xray x25519 -i` (все форматы вывода).
func ParsePublicKey(output string) (string, error) {
	for _, line := range strings.Split(output, "\n") {
		head, value, _ := strings.Cut(line, ":")
		head = strings.ToLower(strings.ReplaceAll(head, " ", ""))
		if strings.Contains(head, "publickey") && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value), nil
		}
	}
	return "", configErr("Не удалось получить публичный ключ из вывода xray x25519")
}

package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"xraypanel/internal/auth"
	"xraypanel/internal/store"
	"xraypanel/internal/xray"
)

// Операции с ключами: БД и Xray меняются согласованно.

const importedKeyName = "Импортирован с сервера"

func newUUID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	b[6] = b[6]&0x0f | 0x40 // версия 4
	b[8] = b[8]&0x3f | 0x80 // вариант RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func ptr[T any](v T) *T { return &v }

// provisionKey создаёт ключ: сначала Xray (если упал — в БД ничего), затем запись в БД
// (если не удалась — ключ удаляется из Xray).
func (a *App) provisionKey(ctx context.Context, owner *store.User, name string, actor *store.User, action string) (*store.VpnKey, error) {
	keyUUID, err := newUUID()
	if err != nil {
		return nil, err
	}
	sni, publicKey, shortID, err := a.xray.RealityParams()
	if err != nil {
		return nil, err
	}
	links, err := a.xray.Links(keyUUID, name)
	if err != nil {
		return nil, err
	}
	if err := a.xray.AddKey(keyUUID); err != nil {
		return nil, err
	}
	key := &store.VpnKey{
		OwnerID:       owner.ID,
		UUID:          keyUUID,
		Name:          name,
		Type:          store.KeyTypeMulti,
		SNI:           sni,
		Status:        store.KeyActive,
		PublicKey:     publicKey,
		ShortID:       shortID,
		ConnectionURI: links[0].URI,
	}
	err = a.st.Tx(ctx, func(q *store.Queries) error {
		if err := q.InsertKey(ctx, key); err != nil {
			return err
		}
		return q.InsertAudit(ctx, store.Audit{
			ActorID: &actor.ID, Action: action, TargetType: "vpn_key", TargetID: fmt.Sprint(key.ID),
			Metadata: map[string]any{"uuid": keyUUID, "owner_id": owner.ID, "name": name},
		})
	})
	if err != nil {
		a.removeQuietly(keyUUID)
		return nil, err
	}
	return key, nil
}

func (a *App) removeQuietly(keyUUID string) {
	if err := a.xray.RemoveKey(keyUUID); err != nil {
		a.log.Error("Ключ остался в Xray без записи в БД", "uuid", keyUUID, "error", err)
	}
}

func (a *App) revokeKey(ctx context.Context, key *store.VpnKey, actor *store.User, action string) error {
	if key.Status == store.KeyRevoked {
		return nil
	}
	if key.Status == store.KeyActive {
		if err := a.xray.RemoveKey(key.UUID); err != nil {
			return err
		}
	}
	return a.st.Tx(ctx, func(q *store.Queries) error {
		if err := q.SetKeyStatus(ctx, key.ID, store.KeyRevoked); err != nil {
			return err
		}
		return q.InsertAudit(ctx, store.Audit{
			ActorID: &actor.ID, Action: action, TargetType: "vpn_key", TargetID: fmt.Sprint(key.ID),
			Metadata: map[string]any{"uuid": key.UUID, "owner_id": key.OwnerID},
		})
	})
}

// suspendUser отключает активные ключи в Xray и помечает пользователя заблокированным.
func (a *App) suspendUser(ctx context.Context, admin, user *store.User, extra map[string]any) (*store.User, error) {
	keys, err := a.st.KeysByOwnerStatus(ctx, user.ID, store.KeyActive)
	if err != nil {
		return nil, err
	}
	for _, key := range keys {
		if err := a.xray.RemoveKey(key.UUID); err != nil {
			return nil, err
		}
	}
	return a.setUserStatus(ctx, admin, user, store.UserSuspended, "user.suspend", "keys_disabled", keys, store.KeySuspended, extra)
}

// activateUser возвращает в Xray ключи, отключённые блокировкой, и подтверждает пользователя.
// При подтверждении заявки назначается тариф по умолчанию (если он выбран). Если подписка
// уже закончилась, ключи не включаются, а переходят в expired — их вернёт продление.
func (a *App) activateUser(ctx context.Context, admin, user *store.User, action string, extra map[string]any) (*store.User, error) {
	if action == "user.approve" && user.PlanID == nil {
		plan, err := a.st.DefaultPlan(ctx)
		switch {
		case err == nil:
			user, err = a.changeSubscription(ctx, admin, user, subscriptionChange{
				SetPlan: true, PlanID: &plan.ID, SetPaid: true, PaidUntil: planPaidUntil(plan, time.Now()),
			}, "subscription.assign", map[string]any{"default_plan": true})
			if err != nil {
				return nil, err
			}
		case !errors.Is(err, store.ErrNotFound):
			return nil, err
		}
	}
	keys, err := a.st.KeysByOwnerStatus(ctx, user.ID, store.KeySuspended)
	if err != nil {
		return nil, err
	}
	if !user.SubscriptionActive(time.Now()) {
		return a.setUserStatus(ctx, admin, user, store.UserApproved, action, "keys_expired", keys, store.KeyExpired, extra)
	}
	for _, key := range keys {
		if err := a.xray.AddKey(key.UUID); err != nil {
			return nil, err
		}
	}
	return a.setUserStatus(ctx, admin, user, store.UserApproved, action, "keys_restored", keys, store.KeyActive, extra)
}

func (a *App) setUserStatus(ctx context.Context, admin, user *store.User, userStatus, action, counter string,
	keys []store.VpnKey, keyStatus string, extra map[string]any) (*store.User, error) {
	meta := map[string]any{counter: len(keys)}
	for k, v := range extra {
		meta[k] = v
	}
	err := a.st.Tx(ctx, func(q *store.Queries) error {
		for _, key := range keys {
			if err := q.SetKeyStatus(ctx, key.ID, keyStatus); err != nil {
				return err
			}
		}
		if err := q.SetUserStatus(ctx, user.ID, userStatus); err != nil {
			return err
		}
		return q.InsertAudit(ctx, store.Audit{
			ActorID: &admin.ID, Action: action, TargetType: "user", TargetID: fmt.Sprint(user.ID),
			Metadata: meta,
		})
	})
	if err != nil {
		return nil, err
	}
	return a.st.UserByID(ctx, user.ID)
}

// importServerKeys закрепляет за администратором ключи, которые есть в Xray, но не в БД.
func (a *App) importServerKeys(ctx context.Context, admin *store.User) (int, error) {
	if _, err := a.xray.Normalize(); err != nil {
		return 0, err
	}
	serverKeys, err := a.xray.ListServerKeys()
	if err != nil {
		return 0, err
	}
	sni, publicKey, shortID, err := a.xray.RealityParams()
	if err != nil {
		return 0, err
	}
	imported := 0
	err = a.st.Tx(ctx, func(q *store.Queries) error {
		known, err := q.KnownUUIDs(ctx)
		if err != nil {
			return err
		}
		var ids []string
		for _, keyUUID := range serverKeys {
			if known[keyUUID] {
				continue
			}
			name := importedKeyName + " " + firstRunes(keyUUID, 8)
			links, err := a.xray.Links(keyUUID, name)
			if err != nil {
				return err
			}
			key := &store.VpnKey{
				OwnerID: admin.ID, UUID: keyUUID, Name: name, Type: store.KeyTypeMulti, SNI: sni,
				Status: store.KeyActive, PublicKey: publicKey, ShortID: shortID, ConnectionURI: links[0].URI,
			}
			if err := q.InsertKey(ctx, key); err != nil {
				return err
			}
			ids = append(ids, fmt.Sprint(key.ID))
		}
		imported = len(ids)
		if imported == 0 {
			return nil
		}
		return q.InsertAudit(ctx, store.Audit{
			ActorID: &admin.ID, Action: "admin.key.import", TargetType: "vpn_key",
			TargetID: strings.Join(ids, ","), Metadata: map[string]any{"count": imported},
		})
	})
	return imported, err
}

func firstRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		r = r[:n]
	}
	return string(r)
}

// ensureSubToken возвращает токен подписки пользователя, создавая или меняя его.
func (a *App) ensureSubToken(ctx context.Context, user *store.User, rotate bool) (string, error) {
	if !rotate && user.SubToken != nil && *user.SubToken != "" {
		return *user.SubToken, nil
	}
	var raw [24]byte // эквивалент secrets.token_urlsafe(24)
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw[:])
	if err := a.st.SetSubToken(ctx, user.ID, token); err != nil {
		return "", err
	}
	user.SubToken = &token
	return token, nil
}

func (a *App) subscriptionURL(token string) string {
	return strings.TrimRight(a.cfg.PublicBaseURL, "/") + "/sub/" + token
}

func (a *App) isOnline(key *store.VpnKey, now time.Time) bool {
	if key.Status != store.KeyActive || key.LastSeenAt == nil {
		return false
	}
	return now.Sub(*key.LastSeenAt) <= time.Duration(a.cfg.OnlineWindowSeconds)*time.Second
}

// SeedAdmin создаёт администратора из ADMIN_EMAIL/ADMIN_PASSWORD, если его ещё нет.
func SeedAdmin(ctx context.Context, st *store.Store, email, password string) (bool, error) {
	if email == "" || password == "" {
		return false, nil
	}
	email = strings.ToLower(email)
	if _, err := st.UserByEmail(ctx, email); err == nil {
		return false, nil
	} else if !errors.Is(err, store.ErrNotFound) {
		return false, err
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return false, err
	}
	user := &store.User{Email: email, PasswordHash: hash, Role: store.RoleAdmin, Status: store.UserApproved, KeyLimit: 100}
	if err := st.CreateUser(ctx, user); err != nil {
		if errors.Is(err, store.ErrConflict) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ---------- представления (JSON) ----------

type keyLinkOut struct {
	Transport string `json:"transport"`
	Label     string `json:"label"`
	URI       string `json:"uri"`
}

type keyOut struct {
	ID               int64        `json:"id"`
	OwnerID          int64        `json:"owner_id"`
	UUID             string       `json:"uuid"`
	Name             string       `json:"name"`
	Type             string       `json:"type"`
	SNI              string       `json:"sni"`
	Status           string       `json:"status"`
	PublicKey        string       `json:"public_key"`
	ShortID          string       `json:"short_id"`
	ConnectionURI    string       `json:"connection_uri"`
	ClientConfigJSON rawJSON      `json:"client_config_json"`
	CreatedAt        jsonTime     `json:"created_at"`
	TrafficUp        int64        `json:"traffic_up"`
	TrafficDown      int64        `json:"traffic_down"`
	LastSeenAt       *jsonTime    `json:"last_seen_at"`
	Links            []keyLinkOut `json:"links"`
	Online           bool         `json:"online"`
	OwnerEmail       *string      `json:"owner_email"`
}

// keyOut строит представление ключа; у отозванных ключей ссылок нет, при ошибке Xray — пустой список.
func (a *App) keyOut(key *store.VpnKey, ownerEmail *string) keyOut {
	links := []keyLinkOut{}
	if key.Status != store.KeyRevoked {
		built, err := a.xray.Links(key.UUID, key.Name)
		var gw *xray.GatewayError
		switch {
		case errors.As(err, &gw):
			a.log.Warn("Не удалось построить ссылки ключа", "key", key.ID, "error", gw.Detail)
		case err != nil:
			a.log.Warn("Не удалось построить ссылки ключа", "key", key.ID, "error", err)
		default:
			for _, l := range built {
				links = append(links, keyLinkOut{Transport: l.Transport, Label: l.Label, URI: l.URI})
			}
		}
	}
	out := keyOut{
		ID: key.ID, OwnerID: key.OwnerID, UUID: key.UUID, Name: key.Name, Type: key.Type, SNI: key.SNI,
		Status: key.Status, PublicKey: key.PublicKey, ShortID: key.ShortID, ConnectionURI: key.ConnectionURI,
		CreatedAt: jsonTime{key.CreatedAt}, TrafficUp: key.TrafficUp, TrafficDown: key.TrafficDown,
		LastSeenAt: toJSONTime(key.LastSeenAt), Links: links, Online: a.isOnline(key, time.Now()), OwnerEmail: ownerEmail,
	}
	if key.ClientConfigJSON != nil && json.Valid([]byte(*key.ClientConfigJSON)) {
		out.ClientConfigJSON = rawJSON(*key.ClientConfigJSON)
	}
	return out
}

type userOut struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Role      string    `json:"role"`
	Status    string    `json:"status"`
	KeyLimit  int       `json:"key_limit"`
	PlanID    *int64    `json:"plan_id"`
	PaidUntil *jsonTime `json:"paid_until"`
	// SubscriptionActive — срок не вышел (или подписка бессрочная).
	SubscriptionActive bool     `json:"subscription_active"`
	CreatedAt          jsonTime `json:"created_at"`
}

func toUserOut(u *store.User) userOut {
	return userOut{
		ID: u.ID, Email: u.Email, Role: u.Role, Status: u.Status, KeyLimit: u.KeyLimit, PlanID: u.PlanID,
		PaidUntil: toJSONTime(u.PaidUntil), SubscriptionActive: u.SubscriptionActive(time.Now()), CreatedAt: jsonTime{u.CreatedAt},
	}
}

type authSession struct {
	User userOut `json:"user"`
}

type message struct {
	Message string `json:"message"`
}

type subscriptionOut struct {
	URL string `json:"url"`
}

type auditOut struct {
	ID            int64    `json:"id"`
	ActorID       *int64   `json:"actor_id"`
	ActorEmail    *string  `json:"actor_email"`
	Action        string   `json:"action"`
	TargetType    string   `json:"target_type"`
	TargetID      string   `json:"target_id"`
	AuditMetadata rawJSON  `json:"audit_metadata"`
	CreatedAt     jsonTime `json:"created_at"`
}

// rawJSON — готовый JSON-текст из БД; пустое значение выводится как null.
type rawJSON string

func (r rawJSON) MarshalJSON() ([]byte, error) {
	if r == "" {
		return []byte("null"), nil
	}
	return []byte(r), nil
}

// jsonTime — момент времени в JSON: RFC3339 в UTC; доли секунды (6 знаков) только если они не нулевые,
// как у прежней версии (pydantic).
type jsonTime struct{ time.Time }

func (t jsonTime) MarshalJSON() ([]byte, error) {
	utc := t.UTC()
	layout := `"2006-01-02T15:04:05Z"`
	if utc.Nanosecond()/1000 != 0 {
		layout = `"2006-01-02T15:04:05.000000Z"`
	}
	return []byte(utc.Format(layout)), nil
}

func toJSONTime(t *time.Time) *jsonTime {
	if t == nil {
		return nil
	}
	return &jsonTime{*t}
}

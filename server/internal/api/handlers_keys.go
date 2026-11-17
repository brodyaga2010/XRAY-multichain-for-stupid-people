package api

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"xraypanel/internal/store"
)

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

type keyCreate struct {
	Name *string `json:"name"`
}

func (a *App) listKeys(w http.ResponseWriter, r *http.Request) error {
	user, err := a.approvedUser(r)
	if err != nil {
		return err
	}
	keys, err := a.st.KeysByOwner(r.Context(), user.ID)
	if err != nil {
		return err
	}
	out := make([]keyOut, 0, len(keys))
	for i := range keys {
		out = append(out, a.keyOut(&keys[i], nil))
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

func (a *App) createKey(w http.ResponseWriter, r *http.Request) error {
	user, err := a.approvedUser(r)
	if err != nil {
		return err
	}
	var body keyCreate
	if err := decodeBody(w, r, &body); err != nil {
		return err
	}
	name, err := keyName(body.Name)
	if err != nil {
		return err
	}
	ctx := detached(r)
	active, err := a.st.CountActiveKeys(ctx, user.ID)
	if err != nil {
		return err
	}
	if active >= user.KeyLimit {
		return httpErr(http.StatusConflict, "Достигнут лимит ключей")
	}
	key, err := a.provisionKey(ctx, user, name, user, "key.create")
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusCreated, a.keyOut(key, nil))
	return nil
}

func (a *App) revokeOwnKey(w http.ResponseWriter, r *http.Request) error {
	user, err := a.approvedUser(r)
	if err != nil {
		return err
	}
	id, err := pathID(r, "key_id")
	if err != nil {
		return err
	}
	ctx := detached(r)
	key, err := a.st.KeyByID(ctx, id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && key.OwnerID != user.ID) {
		return httpErr(http.StatusNotFound, "Ключ не найден")
	}
	if err != nil {
		return err
	}
	if err := a.revokeKey(ctx, key, user, "key.revoke"); err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, message{Message: "Ключ отозван"})
	return nil
}

func (a *App) getSubscription(w http.ResponseWriter, r *http.Request) error {
	return a.subscriptionLink(w, r, false)
}

func (a *App) rotateSubscription(w http.ResponseWriter, r *http.Request) error {
	return a.subscriptionLink(w, r, true)
}

func (a *App) subscriptionLink(w http.ResponseWriter, r *http.Request, rotate bool) error {
	user, err := a.approvedUser(r)
	if err != nil {
		return err
	}
	token, err := a.ensureSubToken(detached(r), user, rotate)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, subscriptionOut{URL: a.subscriptionURL(token)})
	return nil
}

// subscription отдаёт подписку для клиентов (Hiddify, v2rayNG, v2rayN, Streisand, Happ): base64 со ссылками.
func (a *App) subscription(w http.ResponseWriter, r *http.Request) error {
	token := r.PathValue("token")
	if len(token) < 16 {
		return httpErr(http.StatusNotFound, "Not found")
	}
	ctx := r.Context()
	user, err := a.st.UserBySubToken(ctx, token)
	if errors.Is(err, store.ErrNotFound) || (err == nil && user.Status != store.UserApproved) {
		return httpErr(http.StatusNotFound, "Not found")
	}
	if err != nil {
		return err
	}
	keys, err := a.st.ActiveKeysOldestFirst(ctx, user.ID)
	if err != nil {
		return err
	}
	var uris []string
	var upload, download int64
	for _, key := range keys {
		links, err := a.xray.Links(key.UUID, key.Name)
		if err != nil {
			return err
		}
		for _, link := range links {
			uris = append(uris, link.URI)
		}
		upload += key.TrafficUp
		download += key.TrafficDown
	}
	body := base64.StdEncoding.EncodeToString([]byte(strings.Join(uris, "\n")))
	title := base64.StdEncoding.EncodeToString([]byte(a.cfg.SubscriptionTitle))
	h := w.Header()
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("profile-title", "base64:"+title)
	h.Set("profile-update-interval", strconv.Itoa(a.cfg.SubscriptionUpdateHours))
	h.Set("subscription-userinfo", "upload="+itoa(upload)+"; download="+itoa(download)+"; total=0; expire=0")
	h.Set("content-disposition", `attachment; filename="vpn.txt"`)
	h.Set("cache-control", "no-store")
	_, _ = w.Write([]byte(body))
	return nil
}

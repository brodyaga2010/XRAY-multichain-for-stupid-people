package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"xraypanel/internal/store"
)

const dayLayout = "2006-01-02"

type trafficDayOut struct {
	Day  string `json:"day"`
	Up   int64  `json:"up"`
	Down int64  `json:"down"`
}

// parseDays читает days — длину периода в днях (1..366, по умолчанию 30).
func parseDays(r *http.Request) (int, error) {
	raw := r.URL.Query().Get("days")
	if raw == "" {
		return 30, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 366 {
		return 0, invalid("Параметр days должен быть целым числом от 1 до 366")
	}
	return n, nil
}

// fillDays раскладывает строки по дням периода, заканчивающегося днём today; дни без трафика — нули.
func fillDays(rows []store.TrafficDay, today time.Time, days int) []trafficDayOut {
	byDay := make(map[string]store.TrafficDay, len(rows))
	for _, row := range rows {
		byDay[row.Day] = row
	}
	out := make([]trafficDayOut, 0, days)
	for i := days - 1; i >= 0; i-- {
		day := today.AddDate(0, 0, -i).Format(dayLayout)
		row := byDay[day]
		out = append(out, trafficDayOut{Day: day, Up: row.Up, Down: row.Down})
	}
	return out
}

// queryID читает необязательный параметр-идентификатор (0 — не задан).
func queryID(r *http.Request, name, msg string) (int64, error) {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id < 1 {
		return 0, invalid(msg)
	}
	return id, nil
}

// trafficKey читает key_id и проверяет, что ключ есть и (при ownerID > 0) принадлежит этому пользователю.
func (a *App) trafficKey(r *http.Request, ownerID int64) (int64, error) {
	id, err := queryID(r, "key_id", "Параметр key_id должен быть id ключа")
	if err != nil || id == 0 {
		return 0, err
	}
	key, err := a.st.KeyByID(r.Context(), id)
	if errors.Is(err, store.ErrNotFound) || (err == nil && ownerID > 0 && key.OwnerID != ownerID) {
		return 0, httpErr(http.StatusNotFound, "Ключ не найден")
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

// trafficSeries — трафик по дням за последние days дней (по часовому поясу сервера); нулевые поля фильтра
// (кроме дат) — без ограничения.
func (a *App) trafficSeries(ctx context.Context, f store.TrafficFilter, days int) ([]trafficDayOut, error) {
	today := time.Now().In(time.Local)
	f.From, f.To = today.AddDate(0, 0, -(days-1)).Format(dayLayout), today.Format(dayLayout)
	rows, err := a.st.TrafficByDay(ctx, f)
	if err != nil {
		return nil, err
	}
	return fillDays(rows, today, days), nil
}

// adminTraffic — трафик всех ключей, одного пользователя (user_id) или одного ключа (key_id) по дням.
func (a *App) adminTraffic(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminUser(r); err != nil {
		return err
	}
	days, err := parseDays(r)
	if err != nil {
		return err
	}
	ownerID, err := queryID(r, "user_id", "Параметр user_id должен быть id пользователя")
	if err != nil {
		return err
	}
	if ownerID > 0 {
		if _, err := a.getUser(r, ownerID); err != nil {
			return err
		}
	}
	keyID, err := a.trafficKey(r, ownerID)
	if err != nil {
		return err
	}
	out, err := a.trafficSeries(r.Context(), store.TrafficFilter{OwnerID: ownerID, KeyID: keyID}, days)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// ownTraffic — трафик ключей текущего пользователя (или одного своего ключа, key_id) по дням.
func (a *App) ownTraffic(w http.ResponseWriter, r *http.Request) error {
	user, err := a.approvedUser(r)
	if err != nil {
		return err
	}
	days, err := parseDays(r)
	if err != nil {
		return err
	}
	keyID, err := a.trafficKey(r, user.ID)
	if err != nil {
		return err
	}
	out, err := a.trafficSeries(r.Context(), store.TrafficFilter{OwnerID: user.ID, KeyID: keyID}, days)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

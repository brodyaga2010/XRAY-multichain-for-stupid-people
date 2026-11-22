package api

import (
	"context"
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

// trafficSeries — трафик по дням за последние days дней (по часовому поясу сервера); ownerID 0 — все ключи.
func (a *App) trafficSeries(ctx context.Context, ownerID int64, days int) ([]trafficDayOut, error) {
	today := time.Now().In(time.Local)
	rows, err := a.st.TrafficByDay(ctx, store.TrafficFilter{
		From: today.AddDate(0, 0, -(days - 1)).Format(dayLayout), To: today.Format(dayLayout), OwnerID: ownerID,
	})
	if err != nil {
		return nil, err
	}
	return fillDays(rows, today, days), nil
}

// adminTraffic — трафик всех ключей или одного пользователя (user_id) по дням.
func (a *App) adminTraffic(w http.ResponseWriter, r *http.Request) error {
	if _, err := a.adminUser(r); err != nil {
		return err
	}
	days, err := parseDays(r)
	if err != nil {
		return err
	}
	var ownerID int64
	if raw := r.URL.Query().Get("user_id"); raw != "" {
		ownerID, err = strconv.ParseInt(raw, 10, 64)
		if err != nil || ownerID < 1 {
			return invalid("Параметр user_id должен быть id пользователя")
		}
		if _, err := a.getUser(r, ownerID); err != nil {
			return err
		}
	}
	out, err := a.trafficSeries(r.Context(), ownerID, days)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

// ownTraffic — трафик ключей текущего пользователя по дням.
func (a *App) ownTraffic(w http.ResponseWriter, r *http.Request) error {
	user, err := a.approvedUser(r)
	if err != nil {
		return err
	}
	days, err := parseDays(r)
	if err != nil {
		return err
	}
	out, err := a.trafficSeries(r.Context(), user.ID, days)
	if err != nil {
		return err
	}
	writeJSON(w, http.StatusOK, out)
	return nil
}

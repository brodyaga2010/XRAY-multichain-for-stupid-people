package store

import (
	"context"
	"testing"
	"time"
)

func TestMonitorPoints(t *testing.T) {
	ctx := context.Background()
	st := openMigrated(t)
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	ms := 120.5
	for i, p := range []MonitorPoint{
		{At: base, Ms: &ms, Status: "ok"},
		{At: base.Add(time.Minute), Status: "fail"},
		{At: base.Add(2 * time.Minute), Ms: &ms, Status: "warn"},
		{At: base.Add(2 * time.Minute), Ms: &ms, Status: "ok"}, // тот же момент — перезапись
	} {
		if err := st.SaveMonitorPoint(ctx, p); err != nil {
			t.Fatalf("точка %d: %v", i, err)
		}
	}

	tests := []struct {
		name       string
		since      time.Time
		wantLen    int
		wantFirst  string
		wantLastOK bool
	}{
		{"все", base.Add(-time.Hour), 3, "ok", true},
		{"с минуты", base.Add(time.Minute), 2, "fail", true},
		{"из будущего", base.Add(time.Hour), 0, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := st.MonitorPointsSince(ctx, tt.since)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.wantLen {
				t.Fatalf("точек %d, want %d", len(got), tt.wantLen)
			}
			if tt.wantLen > 0 && (got[0].Status != tt.wantFirst || (got[len(got)-1].Status == "ok") != tt.wantLastOK) {
				t.Errorf("точки = %+v", got)
			}
		})
	}

	all, _ := st.MonitorPointsSince(ctx, base.Add(-time.Hour))
	if all[0].Ms == nil || *all[0].Ms != ms || all[1].Ms != nil || !all[0].At.Equal(base) {
		t.Errorf("значения не сохранились: %+v", all[:2])
	}
	if err := st.DeleteMonitorPointsBefore(ctx, base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if left, _ := st.MonitorPointsSince(ctx, base.Add(-time.Hour)); len(left) != 2 {
		t.Errorf("после очистки %d точек, want 2", len(left))
	}
}

func TestMonitorIncidents(t *testing.T) {
	ctx := context.Background()
	st := openMigrated(t)
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	end := base.Add(5 * time.Minute)

	steps := []struct {
		name string
		inc  MonitorIncident
	}{
		{"открыт первый", MonitorIncident{CheckID: "chain", Name: "Цепочка", Status: "fail", Detail: "нет ответа", Start: base}},
		{"открыт второй", MonitorIncident{CheckID: "exit", Name: "Выход", Status: "warn", Start: base.Add(time.Minute)}},
		{"закрыт первый", MonitorIncident{CheckID: "chain", Name: "Цепочка", Status: "fail", Detail: "нет ответа", Start: base, End: &end}},
	}
	for _, s := range steps {
		if err := st.SaveMonitorIncident(ctx, s.inc); err != nil {
			t.Fatalf("%s: %v", s.name, err)
		}
	}

	tests := []struct {
		name      string
		limit     int
		wantIDs   []string
		wantEnded []bool
	}{
		{"все, старые первыми", 10, []string{"chain", "exit"}, []bool{true, false}},
		{"только последний", 1, []string{"exit"}, []bool{false}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := st.LastMonitorIncidents(ctx, tt.limit)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != len(tt.wantIDs) {
				t.Fatalf("сбоев %d: %+v", len(got), got)
			}
			for i := range got {
				if got[i].CheckID != tt.wantIDs[i] || (got[i].End != nil) != tt.wantEnded[i] {
					t.Errorf("сбой %d = %+v", i, got[i])
				}
			}
		})
	}
	got, _ := st.LastMonitorIncidents(ctx, 10)
	if !got[0].End.Equal(end) || got[0].Detail != "нет ответа" || !got[0].Start.Equal(base) {
		t.Errorf("первый сбой = %+v", got[0])
	}
}

func TestMonitorIncidentsKeepLast100(t *testing.T) {
	ctx := context.Background()
	st := openMigrated(t)
	base := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 105; i++ {
		if err := st.SaveMonitorIncident(ctx, MonitorIncident{CheckID: "c", Name: "c", Status: "fail", Start: base.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatal(err)
		}
	}
	got, _ := st.LastMonitorIncidents(ctx, 1000)
	if len(got) != 100 || !got[0].Start.Equal(base.Add(5*time.Minute)) {
		t.Errorf("хранится %d, первый с %v", len(got), got[0].Start)
	}
}

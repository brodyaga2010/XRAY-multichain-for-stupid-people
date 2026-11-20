package stats

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"xraypanel/internal/config"
	"xraypanel/internal/store"
	"xraypanel/internal/xray"
)

func setup(t *testing.T) (*store.Store, *xray.Manager, *xray.MemoryRunner, *store.VpnKey) {
	t.Helper()
	env := map[string]string{"JWT_SECRET": "test-secret-that-is-long-enough", "XRAY_MODE": "dry_run"}
	cfg, err := config.Load(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	if err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(filepath.Join(t.TempDir(), "panel.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	if _, err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	owner := &store.User{Email: "a@example.com", PasswordHash: "h", Role: store.RoleUser, Status: store.UserApproved, KeyLimit: 3}
	if err := st.CreateUser(ctx, owner); err != nil {
		t.Fatal(err)
	}
	key := &store.VpnKey{OwnerID: owner.ID, UUID: "k-1", Name: "k", Type: "multi", SNI: "s", Status: store.KeyActive,
		PublicKey: "p", ShortID: "s", ConnectionURI: "vless://k-1"}
	if err := st.InsertKey(ctx, key); err != nil {
		t.Fatal(err)
	}
	manager, err := xray.Build(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return st, manager, manager.Runner().(*xray.MemoryRunner), key
}

func TestCollectOnceTrafficWithoutOnlineStillMarksSeen(t *testing.T) {
	st, manager, runner, key := setup(t)
	runner.Stats = map[string]int64{"user>>>k-1>>>traffic>>>downlink": 7}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	updated, err := CollectOnce(context.Background(), st, manager, now)

	got, _ := st.KeyByID(context.Background(), key.ID)
	if err != nil || updated != 1 || got.TrafficDown != 7 || got.LastSeenAt == nil || !got.LastSeenAt.Equal(now) {
		t.Errorf("updated=%d err=%v key=%+v", updated, err, got)
	}
}

func TestCollectOnceNothingToDo(t *testing.T) {
	st, manager, _, _ := setup(t)

	updated, err := CollectOnce(context.Background(), st, manager, time.Now())

	if err != nil || updated != 0 {
		t.Errorf("updated=%d err=%v", updated, err)
	}
}

func TestRunCollectsPeriodicallyAndStopsOnCancel(t *testing.T) {
	st, manager, runner, key := setup(t)
	runner.Online = map[string]bool{"k-1": true}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		Run(ctx, st, manager, 10*time.Millisecond)
	}()

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if got, _ := st.KeyByID(context.Background(), key.ID); got.LastSeenAt != nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run не остановился после отмены контекста")
	}
	if got, _ := st.KeyByID(context.Background(), key.ID); got.LastSeenAt == nil {
		t.Error("фоновый сбор не отметил активность")
	}
}

func TestCollectOnceWritesDailyHistory(t *testing.T) {
	st, manager, runner, _ := setup(t)
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	day := now.In(time.Local).Format("2006-01-02")
	ctx := context.Background()

	for _, down := range []int64{7, 3} {
		runner.Stats = map[string]int64{"user>>>k-1>>>traffic>>>downlink": down, "user>>>k-1>>>traffic>>>uplink": 1}
		if _, err := CollectOnce(ctx, st, manager, now); err != nil {
			t.Fatal(err)
		}
	}

	got, err := st.TrafficByDay(ctx, store.TrafficFilter{From: day, To: day})
	if err != nil || len(got) != 1 || got[0].Down != 10 || got[0].Up != 2 {
		t.Errorf("история = %+v, err %v", got, err)
	}
}

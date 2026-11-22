package store

import (
	"context"
	"reflect"
	"testing"
)

func TestTrafficByDay(t *testing.T) {
	ctx := context.Background()
	st := openMigrated(t)
	anna := &User{Email: "anna@example.com", PasswordHash: "h", Role: RoleUser, Status: UserApproved, KeyLimit: 3}
	boris := &User{Email: "boris@example.com", PasswordHash: "h", Role: RoleUser, Status: UserApproved, KeyLimit: 3}
	for _, u := range []*User{anna, boris} {
		if err := st.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	for _, k := range []struct {
		owner *User
		uuid  string
	}{{anna, "a-1"}, {anna, "a-2"}, {boris, "b-1"}} {
		key := &VpnKey{OwnerID: k.owner.ID, UUID: k.uuid, Name: k.uuid, Type: "multi", SNI: "s", Status: KeyActive,
			PublicKey: "p", ShortID: "s", ConnectionURI: "vless://" + k.uuid}
		if err := st.InsertKey(ctx, key); err != nil {
			t.Fatal(err)
		}
	}
	for _, add := range []struct {
		uuid     string
		up, down int64
		day      string
	}{
		{"a-1", 10, 100, "2026-11-18"},
		{"a-1", 5, 50, "2026-11-18"}, // тот же день — складывается
		{"a-2", 1, 1, "2026-11-19"},
		{"b-1", 7, 70, "2026-11-19"},
		{"b-1", 0, 0, "2026-11-20"}, // пустой сбор — строки не будет
		{"нет-такого", 9, 9, "2026-11-20"},
	} {
		if _, err := st.AddTraffic(ctx, add.uuid, add.up, add.down, nil, add.day); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name string
		f    TrafficFilter
		want []TrafficDay
	}{
		{"все пользователи", TrafficFilter{From: "2026-11-01", To: "2026-11-30"},
			[]TrafficDay{{"2026-11-18", 15, 150}, {"2026-11-19", 8, 71}}},
		{"только anna", TrafficFilter{From: "2026-11-01", To: "2026-11-30", OwnerID: anna.ID},
			[]TrafficDay{{"2026-11-18", 15, 150}, {"2026-11-19", 1, 1}}},
		{"границы включительно", TrafficFilter{From: "2026-11-19", To: "2026-11-19"},
			[]TrafficDay{{"2026-11-19", 8, 71}}},
		{"пустой период", TrafficFilter{From: "2026-12-01", To: "2026-12-31"}, []TrafficDay{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := st.TrafficByDay(ctx, tt.f)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %+v, want %+v", got, tt.want)
			}
		})
	}
}

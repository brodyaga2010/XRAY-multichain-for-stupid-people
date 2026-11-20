package main

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"xraypanel/internal/api"
)

type fakeExpirer struct {
	out []api.ExpiredSubscription
	err error
}

func (f fakeExpirer) ExpireSubscriptions(context.Context) ([]api.ExpiredSubscription, error) {
	return f.out, f.err
}

type alerts []string

func (a *alerts) Alert(_ context.Context, text string) { *a = append(*a, text) }

func TestExpireSubscriptionsAlerts(t *testing.T) {
	tests := []struct {
		name    string
		expirer fakeExpirer
		want    []string
	}{
		{"никто не истёк", fakeExpirer{}, nil},
		{"двое истекли", fakeExpirer{out: []api.ExpiredSubscription{{Email: "a@example.com", KeysDisabled: 2}, {Email: "b@example.com", KeysDisabled: 1}}},
			[]string{"⏰ Закончилась подписка: a@example.com — отключено ключей: 2", "⏰ Закончилась подписка: b@example.com — отключено ключей: 1"}},
		{"ошибка у одного, второй обработан", fakeExpirer{out: []api.ExpiredSubscription{{Email: "b@example.com", KeysDisabled: 1}}, err: errors.New("xray")},
			[]string{"⏰ Закончилась подписка: b@example.com — отключено ключей: 1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got alerts
			expireSubscriptions(context.Background(), tt.expirer, &got)
			if !reflect.DeepEqual([]string(got), tt.want) {
				t.Errorf("got %q", got)
			}
		})
	}
	expireSubscriptions(context.Background(), fakeExpirer{out: []api.ExpiredSubscription{{Email: "x"}}}, nil) // без бота не падает
}

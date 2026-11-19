package monitor

import (
	"testing"
	"time"
)

func TestSnapshotPublic(t *testing.T) {
	at := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	ms := 190.0
	state := func(id string, status Status, alerted bool) CheckState {
		r := Result{Status: status}
		if id == CheckChain && status != StatusFail {
			r.Ms = &ms
		}
		return CheckState{ID: id, Result: r, Alerted: alerted}
	}
	tests := []struct {
		name   string
		snap   Snapshot
		want   Status
		wantMs bool
	}{
		{"мониторинг выключен", Snapshot{}, StatusUnknown, false},
		{"первая проверка не прошла", Snapshot{Enabled: true}, StatusUnknown, false},
		{"всё работает", Snapshot{Enabled: true, CheckedAt: at, Checks: []CheckState{
			state(CheckChain, StatusOK, false), state(CheckExit, StatusOK, false), state(CheckXray, StatusOK, false)}}, StatusOK, true},
		{"разовый сбой ниже порога не виден", Snapshot{Enabled: true, CheckedAt: at, Checks: []CheckState{
			state(CheckChain, StatusFail, false), state(CheckExit, StatusOK, false)}}, StatusOK, false},
		{"подтверждённые перебои канала", Snapshot{Enabled: true, CheckedAt: at, Checks: []CheckState{
			state(CheckChain, StatusOK, false), state(CheckExit, StatusWarn, true)}}, StatusWarn, true},
		{"сбой цепочки важнее перебоев", Snapshot{Enabled: true, CheckedAt: at, Checks: []CheckState{
			state(CheckChain, StatusFail, true), state(CheckExit, StatusWarn, true)}}, StatusFail, false},
		{"сбой не закрыт, но проверка недоступна", Snapshot{Enabled: true, CheckedAt: at, Checks: []CheckState{
			state(CheckXray, StatusUnknown, true)}}, StatusWarn, false},
		{"сертификаты и диск пользователям не важны", Snapshot{Enabled: true, CheckedAt: at, Checks: []CheckState{
			state(CheckChain, StatusOK, false), state(CheckCerts, StatusFail, true), state(CheckSystem, StatusFail, true)}}, StatusOK, true},
		{"dry_run: всё неизвестно", Snapshot{Enabled: true, CheckedAt: at, Checks: []CheckState{
			state(CheckChain, StatusUnknown, false), state(CheckSystem, StatusOK, false)}}, StatusUnknown, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.snap.Public()
			if got.Status != tt.want || (got.Ms != nil) != tt.wantMs {
				t.Errorf("got %+v, want %s ms=%v", got, tt.want, tt.wantMs)
			}
		})
	}
}

package backup

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeSource struct {
	content string
	err     error
	path    string
}

func (f *fakeSource) BackupTo(_ context.Context, path string) error {
	f.path = path
	if f.err != nil {
		return f.err
	}
	return os.WriteFile(path, []byte(f.content), 0o600)
}

type fakeSender struct {
	err      error
	filename string
	data     []byte
	caption  string
	alerts   []string
}

func (f *fakeSender) SendBackup(_ context.Context, filename string, data []byte, caption string) error {
	f.filename, f.data, f.caption = filename, data, caption
	return f.err
}

func (f *fakeSender) Alert(_ context.Context, text string) { f.alerts = append(f.alerts, text) }

func gunzip(t *testing.T, data []byte) string {
	t.Helper()
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	out, err := io.ReadAll(zr)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestRunOnce(t *testing.T) {
	at := time.Date(2026, 11, 20, 4, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		label        string
		source       *fakeSource
		sender       *fakeSender
		wantErr      string
		wantFilename string
	}{
		{"копия отправлена", "kemsekov", &fakeSource{content: "SQLite format 3"}, &fakeSender{}, "", "kemsekov-panel-2026-11-20-0400.db.gz"},
		{"без имени сайта", "", &fakeSource{content: "db"}, &fakeSender{}, "", "panel-2026-11-20-0400.db.gz"},
		{"копия не снялась", "x", &fakeSource{err: errors.New("disk full")}, &fakeSender{}, "disk full", ""},
		{"не отправилась", "x", &fakeSource{content: "db"}, &fakeSender{err: errors.New("telegram down")}, "telegram down", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			b := New(tt.source, tt.sender, dir, tt.label)
			b.now = func() time.Time { return at }

			size, err := b.RunOnce(context.Background())

			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if tt.sender.filename != tt.wantFilename || size != len(tt.sender.data) {
					t.Errorf("filename = %q size = %d", tt.sender.filename, size)
				}
				if got := gunzip(t, tt.sender.data); got != tt.source.content {
					t.Errorf("содержимое = %q", got)
				}
				if !strings.Contains(tt.sender.caption, "20.11.2026 04:00") {
					t.Errorf("caption = %q", tt.sender.caption)
				}
			}
			if filepath.Dir(tt.source.path) != dir {
				t.Errorf("временный файл не в каталоге базы: %s", tt.source.path)
			}
			if _, err := os.Stat(tt.source.path); !os.IsNotExist(err) {
				t.Errorf("временный файл не удалён: %v", err)
			}
		})
	}
}

func TestNextRun(t *testing.T) {
	msk := time.FixedZone("MSK", 3*3600)
	tests := []struct {
		name string
		now  time.Time
		hour int
		want time.Time
	}{
		{"до часа — сегодня", time.Date(2026, 11, 20, 2, 30, 0, 0, msk), 4, time.Date(2026, 11, 20, 4, 0, 0, 0, msk)},
		{"ровно в час — завтра", time.Date(2026, 11, 20, 4, 0, 0, 0, msk), 4, time.Date(2026, 11, 21, 4, 0, 0, 0, msk)},
		{"после часа — завтра", time.Date(2026, 11, 20, 23, 59, 0, 0, msk), 4, time.Date(2026, 11, 21, 4, 0, 0, 0, msk)},
		{"полночь", time.Date(2026, 12, 31, 12, 0, 0, 0, msk), 0, time.Date(2027, 1, 1, 0, 0, 0, 0, msk)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NextRun(tt.now, tt.hour); !got.Equal(tt.want) {
				t.Errorf("NextRun = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHumanSize(t *testing.T) {
	tests := []struct {
		n    int
		want string
	}{{512, "512 Б"}, {2048, "2 КБ"}, {3 << 20, "3.0 МБ"}}
	for _, tt := range tests {
		if got := humanSize(tt.n); got != tt.want {
			t.Errorf("humanSize(%d) = %q, want %q", tt.n, got, tt.want)
		}
	}
}

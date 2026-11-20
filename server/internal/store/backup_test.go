package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestBackupTo(t *testing.T) {
	ctx := context.Background()
	st := openTemp(t)
	if _, err := st.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := st.CreateUser(ctx, &User{Email: "a@example.com", PasswordHash: "x", Role: RoleUser, Status: UserApproved, KeyLimit: 3}); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(t.TempDir(), "exists.db")
	if err := os.WriteFile(existing, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		path    string
		wantErr bool
	}{
		{"новый файл", filepath.Join(t.TempDir(), "copy.db"), false},
		{"файл уже есть", existing, true},
		{"нет каталога", filepath.Join(t.TempDir(), "нет", "copy.db"), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := st.BackupTo(ctx, tt.path)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			cp, err := Open(tt.path)
			if err != nil {
				t.Fatal(err)
			}
			defer cp.Close()
			users, err := cp.ListUsers(ctx)
			if err != nil || len(users) != 1 || users[0].Email != "a@example.com" {
				t.Fatalf("в копии users = %+v, err = %v", users, err)
			}
		})
	}
}

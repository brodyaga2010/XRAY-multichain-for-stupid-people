package xray

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"
)

// CommandError — команда или файловая операция на стороне Xray не удалась.
type CommandError struct{ Msg string }

func (e *CommandError) Error() string { return e.Msg }

func cmdErr(format string, args ...any) error {
	return &CommandError{Msg: fmt.Sprintf(format, args...)}
}

// Runner — исполнитель команд и файловых операций для менеджера Xray.
type Runner interface {
	Run(args []string, timeout time.Duration) (string, error)
	ReadText(path string) (string, error)
	WriteText(path, content string) error
	Replace(src, dst string) error
	Copy(src, dst string) error
	ListDir(path string) ([]string, error)
	Remove(path string) error
	// Lock берёт эксклюзивную блокировку и возвращает функцию её снятия.
	Lock() (unlock func(), err error)
}

// ---------- LocalRunner ----------

// LocalRunner работает на том же сервере, что и Xray.
type LocalRunner struct {
	lockPath string
	mu       sync.Mutex
}

// NewLocalRunner создаёт локальный исполнитель; lockPath — файл для flock.
func NewLocalRunner(lockPath string) *LocalRunner { return &LocalRunner{lockPath: lockPath} }

func (r *LocalRunner) Run(args []string, timeout time.Duration) (string, error) {
	if len(args) == 0 {
		return "", cmdErr("пустая команда")
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return "", cmdErr("%s: превышено время ожидания (%s)", shellJoin(args), timeout)
		}
		if _, ok := err.(*exec.ExitError); !ok {
			return "", cmdErr("%s: %v", shellJoin(args), err)
		}
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = strings.TrimSpace(stdout.String())
		}
		return "", cmdErr("%s: %s", shellJoin(args), truncateRunes(msg, 500))
	}
	return stdout.String(), nil
}

func (r *LocalRunner) ReadText(path string) (string, error) {
	data, err := os.ReadFile(path)
	return string(data), err
}

func (r *LocalRunner) WriteText(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func (r *LocalRunner) Replace(src, dst string) error {
	if err := os.Chmod(src, 0o644); err != nil { // Xray работает от nobody и должен читать конфиг
		return err
	}
	return os.Rename(src, dst)
}

func (r *LocalRunner) Copy(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

func (r *LocalRunner) ListDir(path string) ([]string, error) {
	entries, err := os.ReadDir(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	return names, nil
}

func (r *LocalRunner) Remove(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (r *LocalRunner) Lock() (func(), error) {
	r.mu.Lock()
	if err := os.MkdirAll(filepath.Dir(r.lockPath), 0o755); err != nil {
		r.mu.Unlock()
		return nil, err
	}
	handle, err := os.OpenFile(r.lockPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		r.mu.Unlock()
		return nil, err
	}
	if err := syscall.Flock(int(handle.Fd()), syscall.LOCK_EX); err != nil {
		handle.Close()
		r.mu.Unlock()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(handle.Fd()), syscall.LOCK_UN)
		_ = handle.Close()
		r.mu.Unlock()
	}, nil
}

// ---------- MemoryRunner ----------

// MemoryRunner — файлы в памяти и имитация `xray` для dry-run и тестов.
// Поля можно менять напрямую из тестов (когда другие горутины не работают).
type MemoryRunner struct {
	Files    map[string]string
	Commands [][]string
	APIUsers map[string]map[string]bool
	Stats    map[string]int64
	Online   map[string]bool
	FailAPI  bool
	FailTest bool

	mu     sync.Mutex // состояние
	lockMu sync.Mutex // Lock()
}

// NewMemoryRunner создаёт исполнитель с начальными файлами.
func NewMemoryRunner(files map[string]string) *MemoryRunner {
	r := &MemoryRunner{
		Files:    map[string]string{},
		APIUsers: map[string]map[string]bool{},
		Stats:    map[string]int64{},
		Online:   map[string]bool{},
	}
	for k, v := range files {
		r.Files[k] = v
	}
	return r
}

func (r *MemoryRunner) Run(args []string, timeout time.Duration) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Commands = append(r.Commands, append([]string(nil), args...))
	if len(args) < 2 {
		return "", nil
	}
	// Первый элемент — путь к бинарнику (любой), дальше подкоманда.
	switch {
	case args[1] == "x25519":
		return "PrivateKey: dry-run-private\nPassword (PublicKey): dry-run-public-key\nHash32: x\n", nil
	case args[1] == "run" && len(args) > 2 && args[2] == "-test":
		last := args[len(args)-1]
		if !strings.HasSuffix(last, ".json") {
			return "", cmdErr("Failed to get format of %s", last)
		}
		if r.FailTest {
			return "", cmdErr("config test failed")
		}
		content, ok := r.Files[last]
		if !ok || !json.Valid([]byte(content)) {
			return "", cmdErr("config test failed: %s", last)
		}
		return "Configuration OK.", nil
	case args[1] == "api" && len(args) > 2:
		return r.api(args[2], args[3:])
	}
	return "", nil
}

func (r *MemoryRunner) api(command string, rest []string) (string, error) {
	if r.FailAPI {
		return "", cmdErr("api unavailable")
	}
	switch command {
	case "adu":
		content, ok := r.Files[rest[len(rest)-1]]
		if !ok {
			return "", cmdErr("adu: файл не найден")
		}
		var payload struct {
			Inbounds []struct {
				Tag      string `json:"tag"`
				Settings struct {
					Clients []struct {
						Email string `json:"email"`
					} `json:"clients"`
				} `json:"settings"`
			} `json:"inbounds"`
		}
		if err := json.Unmarshal([]byte(content), &payload); err != nil {
			return "", cmdErr("adu: %v", err)
		}
		count := 0
		for _, inbound := range payload.Inbounds {
			for _, client := range inbound.Settings.Clients {
				if r.APIUsers[inbound.Tag] == nil {
					r.APIUsers[inbound.Tag] = map[string]bool{}
				}
				r.APIUsers[inbound.Tag][client.Email] = true
				count++
			}
		}
		return fmt.Sprintf("Added %d user(s) in total.\n", count), nil
	case "rmu":
		tag := ""
		for _, arg := range rest {
			if v, ok := strings.CutPrefix(arg, "-tag="); ok {
				tag = v
				break
			}
		}
		email := rest[len(rest)-1]
		removed := 0
		if r.APIUsers[tag][email] {
			removed = 1
			delete(r.APIUsers[tag], email)
		}
		return fmt.Sprintf("Removed %d user(s) in total.\n", removed), nil
	case "statsquery":
		type stat struct {
			Name  string `json:"name"`
			Value int64  `json:"value"`
		}
		names := make([]string, 0, len(r.Stats))
		for name := range r.Stats {
			names = append(names, name)
		}
		sort.Strings(names)
		stats := make([]stat, 0, len(names))
		for _, name := range names {
			stats = append(stats, stat{name, r.Stats[name]})
		}
		for _, arg := range rest {
			if arg == "-reset" {
				r.Stats = map[string]int64{}
			}
		}
		if len(stats) == 0 {
			return "{}", nil
		}
		out, _ := json.Marshal(map[string]any{"stat": stats})
		return string(out), nil
	case "statsgetallonlineusers":
		users := make([]string, 0, len(r.Online))
		for email := range r.Online {
			users = append(users, "user>>>"+email+">>>online")
		}
		sort.Strings(users)
		if len(users) == 0 {
			return "{}", nil
		}
		out, _ := json.Marshal(map[string]any{"users": users})
		return string(out), nil
	}
	return "", nil
}

func (r *MemoryRunner) ReadText(path string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	content, ok := r.Files[path]
	if !ok {
		return "", cmdErr("%s: not found", path)
	}
	return content, nil
}

func (r *MemoryRunner) WriteText(path, content string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Files[path] = content
	return nil
}

func (r *MemoryRunner) Replace(src, dst string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	content, ok := r.Files[src]
	if !ok {
		return cmdErr("%s: not found", src)
	}
	delete(r.Files, src)
	r.Files[dst] = content
	return nil
}

func (r *MemoryRunner) Copy(src, dst string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	content, ok := r.Files[src]
	if !ok {
		return cmdErr("%s: not found", src)
	}
	r.Files[dst] = content
	return nil
}

func (r *MemoryRunner) ListDir(path string) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	prefix := strings.TrimRight(path, "/") + "/"
	var names []string
	for name := range r.Files {
		if strings.HasPrefix(name, prefix) {
			names = append(names, name[len(prefix):])
		}
	}
	sort.Strings(names)
	return names, nil
}

func (r *MemoryRunner) Remove(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.Files, path)
	return nil
}

func (r *MemoryRunner) Lock() (func(), error) {
	r.lockMu.Lock()
	return r.lockMu.Unlock, nil
}

// ---------- утилиты ----------

// shellJoin — аналог shlex.join.
func shellJoin(args []string) string {
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = shellQuote(a)
	}
	return strings.Join(parts, " ")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	safe := true
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || strings.ContainsRune("@%+=:,./-_", c)) {
			safe = false
			break
		}
	}
	if safe {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// ShellSplit — аналог shlex.split (POSIX): пробелы, одинарные и двойные кавычки, обратный слеш.
func ShellSplit(s string) ([]string, error) {
	var (
		out     []string
		cur     strings.Builder
		inToken bool
		quote   rune
	)
	runes := []rune(s)
	for i := 0; i < len(runes); i++ {
		c := runes[i]
		switch {
		case quote == '\'':
			if c == '\'' {
				quote = 0
			} else {
				cur.WriteRune(c)
			}
		case quote == '"':
			switch {
			case c == '"':
				quote = 0
			case c == '\\' && i+1 < len(runes) && strings.ContainsRune("\"\\$`", runes[i+1]):
				i++
				cur.WriteRune(runes[i])
			default:
				cur.WriteRune(c)
			}
		case c == '\'' || c == '"':
			quote, inToken = c, true
		case c == '\\':
			if i+1 >= len(runes) {
				return nil, fmt.Errorf("обратный слеш в конце строки")
			}
			i++
			cur.WriteRune(runes[i])
			inToken = true
		case c == ' ' || c == '\t' || c == '\n':
			if inToken || cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
				inToken = false
			}
		default:
			cur.WriteRune(c)
			inToken = true
		}
	}
	if quote != 0 {
		return nil, fmt.Errorf("нет закрывающей кавычки")
	}
	if inToken || cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out, nil
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

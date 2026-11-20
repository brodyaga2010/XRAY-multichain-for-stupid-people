package monitor

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Deps — откуда проверки берут данные.
type Deps struct {
	// ProbeAddr — служебный SOCKS-вход Xray (127.0.0.1:10811); пусто — проверка цепочки выключена.
	ProbeAddr string
	ProbeURL  string
	// ChainSlow — дольше этого запрос через цепочку считается деградацией.
	ChainSlow time.Duration
	// ExitServer — адрес выходного сервера; ok=false — сервер одиночный.
	ExitServer func() (host string, port int, ok bool, err error)
	ExitSlow   time.Duration
	// XrayOnline — число онлайн-ключей по API Xray (заодно проверка, что Xray жив).
	XrayOnline func() (int, error)
	// CertAddr — куда подключаться за сертификатами ("домен-VPN:443"), CertDomains — какие проверять.
	CertAddr    string
	CertDomains []string
	certRoots   *x509.CertPool // для тестов; nil — системные корневые сертификаты
	// DataDir — по какому разделу считать занятость диска.
	DataDir string
}

// Идентификаторы проверок (используются в API и интерфейсе).
const (
	CheckChain  = "chain"
	CheckExit   = "exit"
	CheckXray   = "xray"
	CheckCerts  = "certs"
	CheckSystem = "system"
)

// DefaultChecks — набор проверок кабинета.
func DefaultChecks(d Deps) []Check {
	return []Check{
		{ID: CheckChain, Name: "Цепочка до интернета", Timeout: 20 * time.Second, History: true, Run: d.chain},
		{ID: CheckExit, Name: "Канал до выходного сервера", Timeout: 15 * time.Second, Run: d.exit},
		{ID: CheckXray, Name: "Xray на входном сервере", Timeout: 25 * time.Second, Run: d.xray},
		{ID: CheckCerts, Name: "Сертификаты HTTPS", Every: time.Hour, Timeout: 20 * time.Second, Run: d.certs},
		{ID: CheckSystem, Name: "Диск и память", Timeout: 5 * time.Second, Run: d.system},
	}
}

// latency — «127 мс», от секунды — «1,25 с». Так же задержку пишет интерфейс (formatLatency).
func latency(d time.Duration) string {
	if d < time.Second {
		return fmt.Sprintf("%d мс", d.Milliseconds())
	}
	return strings.Replace(fmt.Sprintf("%.2f с", d.Seconds()), ".", ",", 1)
}

func ms(d time.Duration) *float64 {
	v := float64(d.Microseconds()) / 1000
	return &v
}

// chain — запрос в интернет через служебный вход: тот же путь, что у трафика пользователей.
func (d Deps) chain(ctx context.Context) Result {
	if d.ProbeAddr == "" {
		return Result{Status: StatusUnknown, Value: "выключено"}
	}
	client := &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
				return dialSOCKS5(ctx, d.ProbeAddr, addr)
			},
			DisableKeepAlives:   true, // каждый раз полный путь: новое соединение через всю цепочку
			TLSHandshakeTimeout: 15 * time.Second,
		},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.ProbeURL, nil)
	if err != nil {
		return Result{Status: StatusFail, Detail: "некорректный MONITOR_PROBE_URL"}
	}
	start := time.Now()
	resp, err := client.Do(req)
	elapsed := time.Since(start)
	if err != nil {
		return Result{Status: StatusFail, Detail: chainError(ctx, err)}
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return Result{Status: StatusFail, Detail: fmt.Sprintf("проверочный сайт ответил %d", resp.StatusCode), Ms: ms(elapsed)}
	}
	r := Result{Status: StatusOK, Value: latency(elapsed), Ms: ms(elapsed)}
	if d.ChainSlow > 0 && elapsed > d.ChainSlow {
		r.Status, r.Detail = StatusWarn, "очень медленно: "+latency(elapsed)
	}
	return r
}

func chainError(ctx context.Context, err error) string {
	switch {
	case errors.Is(err, errProbeUnavailable):
		return errProbeUnavailable.Error()
	case ctx.Err() != nil || errors.Is(err, context.DeadlineExceeded):
		return "нет ответа — трафик не проходит через цепочку"
	case errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF):
		// Xray закрывает соединение, если выходной сервер недоступен или не смог достучаться до сайта.
		return "соединение оборвалось — трафик не проходит через цепочку"
	}
	msg := err.Error()
	if i := strings.LastIndex(msg, ": "); i >= 0 { // url.Error: «Get "https://…": причина»
		msg = msg[i+2:]
	}
	return "запрос не прошёл: " + msg
}

// exit — пять подключений к выходному серверу: задержка и потери на участке между серверами.
func (d Deps) exit(ctx context.Context) Result {
	if d.ExitServer == nil {
		return Result{Status: StatusUnknown}
	}
	host, port, ok, err := d.ExitServer()
	if err != nil {
		return Result{Status: StatusUnknown, Detail: "не удалось прочитать конфиг Xray"}
	}
	if !ok {
		return Result{Status: StatusUnknown, Value: "сервер без выходного узла"}
	}
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	const attempts = 5
	var times []time.Duration
	for i := 0; i < attempts && ctx.Err() == nil; i++ {
		dctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		start := time.Now()
		conn, err := (&net.Dialer{}).DialContext(dctx, "tcp", addr)
		cancel()
		if err == nil {
			times = append(times, time.Since(start))
			conn.Close()
		}
	}
	lost := attempts - len(times)
	if len(times) == 0 {
		return Result{Status: StatusFail, Detail: "нет связи с " + addr}
	}
	sort.Slice(times, func(i, j int) bool { return times[i] < times[j] })
	median := times[len(times)/2]
	r := Result{Status: StatusOK, Value: fmt.Sprintf("%d мс", median.Milliseconds()), Ms: ms(median)}
	switch {
	case lost > 0:
		r.Status, r.Detail = StatusWarn, fmt.Sprintf("потери: %d из %d подключений не прошли", lost, attempts)
	case d.ExitSlow > 0 && median > d.ExitSlow:
		r.Status, r.Detail = StatusWarn, fmt.Sprintf("высокая задержка: %d мс", median.Milliseconds())
	}
	return r
}

func (d Deps) xray(context.Context) Result {
	if d.XrayOnline == nil {
		return Result{Status: StatusUnknown}
	}
	n, err := d.XrayOnline()
	if err != nil {
		return Result{Status: StatusFail, Detail: "API Xray не отвечает — Xray, вероятно, не запущен"}
	}
	return Result{Status: StatusOK, Value: fmt.Sprintf("онлайн: %d", n)}
}

const (
	certWarnDays = 14
	certFailDays = 3
)

// certs — срок действия сертификатов сайта-заглушки и кабинета (Caddy продлевает их сам; тревога — если не смог).
func (d Deps) certs(ctx context.Context) Result {
	if d.CertAddr == "" || len(d.CertDomains) == 0 {
		return Result{Status: StatusUnknown}
	}
	minDays, worst := 1<<30, ""
	for _, domain := range d.CertDomains {
		dialer := &tls.Dialer{Config: &tls.Config{ServerName: domain, RootCAs: d.certRoots}}
		conn, err := dialer.DialContext(ctx, "tcp", d.CertAddr)
		if err != nil {
			return Result{Status: StatusFail, Detail: fmt.Sprintf("%s: %v", domain, err)}
		}
		state := conn.(*tls.Conn).ConnectionState()
		conn.Close()
		if len(state.PeerCertificates) == 0 {
			return Result{Status: StatusFail, Detail: domain + ": нет сертификата"}
		}
		days := int(time.Until(state.PeerCertificates[0].NotAfter).Hours() / 24)
		if days < minDays {
			minDays, worst = days, domain
		}
	}
	r := Result{Status: StatusOK, Value: fmt.Sprintf("действуют ещё %d дн.", minDays)}
	switch {
	case minDays < certFailDays:
		r.Status, r.Detail = StatusFail, fmt.Sprintf("сертификат %s истекает через %d дн.", worst, minDays)
	case minDays < certWarnDays:
		r.Status, r.Detail = StatusWarn, fmt.Sprintf("сертификат %s истекает через %d дн. и не продлён", worst, minDays)
	}
	return r
}

const (
	diskWarnPct = 90
	diskFailPct = 97
	memWarnPct  = 92
)

func (d Deps) system(context.Context) Result {
	var parts []string
	r := Result{Status: StatusOK}
	if d.DataDir != "" {
		var st syscall.Statfs_t
		if err := syscall.Statfs(d.DataDir, &st); err == nil && st.Blocks > 0 {
			used := 100 - int(st.Bavail*100/st.Blocks)
			parts = append(parts, fmt.Sprintf("диск %d%%", used))
			switch {
			case used >= diskFailPct:
				r.Status, r.Detail = StatusFail, fmt.Sprintf("диск заполнен на %d%%", used)
			case used >= diskWarnPct:
				r.Status, r.Detail = StatusWarn, fmt.Sprintf("диск заполнен на %d%%", used)
			}
		}
	}
	if used, ok := memoryUsedPct("/proc/meminfo"); ok {
		parts = append(parts, fmt.Sprintf("память %d%%", used))
		if used >= memWarnPct && r.Status == StatusOK {
			r.Status, r.Detail = StatusWarn, fmt.Sprintf("память занята на %d%%", used)
		}
	}
	if len(parts) == 0 {
		return Result{Status: StatusUnknown}
	}
	r.Value = strings.Join(parts, ", ")
	return r
}

// memoryUsedPct — доля занятой памяти по MemTotal/MemAvailable (только Linux).
func memoryUsedPct(path string) (int, bool) {
	f, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer f.Close()
	values := map[string]int64{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Fields(sc.Text())
		if len(fields) >= 2 {
			if v, err := strconv.ParseInt(fields[1], 10, 64); err == nil {
				values[strings.TrimSuffix(fields[0], ":")] = v
			}
		}
	}
	total, avail := values["MemTotal"], values["MemAvailable"]
	if total <= 0 {
		return 0, false
	}
	return int(100 - avail*100/total), true
}

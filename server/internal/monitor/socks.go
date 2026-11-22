package monitor

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

// errProbeUnavailable — служебный SOCKS-вход Xray не принимает соединения.
var errProbeUnavailable = errors.New("служебный вход Xray для проверки недоступен")

// dialSOCKS5 открывает соединение до target ("host:port") через SOCKS5 без авторизации (RFC 1928).
// Имя хоста передаётся прокси как есть: DNS разрешается на стороне Xray, как у пользователей.
func dialSOCKS5(ctx context.Context, proxyAddr, target string) (net.Conn, error) {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return nil, err
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 || len(host) > 255 {
		return nil, fmt.Errorf("некорректный адрес %q", target)
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", proxyAddr)
	if err != nil {
		return nil, errProbeUnavailable
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	if err := socksHandshake(conn, host, port); err != nil {
		conn.Close()
		return nil, err
	}
	_ = conn.SetDeadline(time.Time{})
	return conn, nil
}

func socksHandshake(conn net.Conn, host string, port int) error {
	if _, err := conn.Write([]byte{5, 1, 0}); err != nil { // версия 5, один метод: без авторизации
		return err
	}
	reply := make([]byte, 2)
	if _, err := io.ReadFull(conn, reply); err != nil {
		return fmt.Errorf("SOCKS: %w", err)
	}
	if reply[0] != 5 || reply[1] != 0 {
		return errors.New("SOCKS: прокси не принял метод без авторизации")
	}

	req := []byte{5, 1, 0, 3, byte(len(host))} // CONNECT, адрес — доменное имя
	req = append(req, host...)
	req = binary.BigEndian.AppendUint16(req, uint16(port))
	if _, err := conn.Write(req); err != nil {
		return err
	}
	head := make([]byte, 4)
	if _, err := io.ReadFull(conn, head); err != nil {
		return fmt.Errorf("SOCKS: %w", err)
	}
	if head[1] != 0 {
		return fmt.Errorf("SOCKS: отказ в соединении (код %d)", head[1])
	}
	var skip int
	switch head[3] {
	case 1:
		skip = 4
	case 4:
		skip = 16
	case 3:
		l := make([]byte, 1)
		if _, err := io.ReadFull(conn, l); err != nil {
			return err
		}
		skip = int(l[0])
	default:
		return fmt.Errorf("SOCKS: неизвестный тип адреса %d", head[3])
	}
	_, err := io.ReadFull(conn, make([]byte, skip+2)) // адрес и порт привязки не нужны
	return err
}

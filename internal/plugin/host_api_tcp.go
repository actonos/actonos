package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"time"

	"github.com/actonos/actonos/internal/security"
	"github.com/tetratelabs/wazero/api"
)

// Return codes of the acton_tcp functions. Non-negative values are results (a handle, a byte
// count); every failure is negative.
const (
	TCPErrNoHost    int32 = -1
	TCPErrDenied    int32 = -2 // manifest does not list host:port, or bad request
	TCPErrBlocked   int32 = -3 // the address is local or private
	TCPErrConnect   int32 = -4 // dial, STARTTLS or handshake failed, or a write failed
	TCPErrTooMany   int32 = -5
	TCPErrOverLimit int32 = -6 // the per-connection byte budget is used up
	TCPErrClosed    int32 = -7 // unknown handle, already closed, or the peer closed
)

const (
	maxTCPConns        = 4
	defaultTCPBytes    = 50 << 20
	tcpIdle            = 60 * time.Second
	maxTCPRecv         = 64 << 10
	maxTCPSend         = 1 << 20
	tcpPreambleTimeout = 10 * time.Second
)

// Connection modes. The plaintext phase of STARTTLS is run by the host with fixed bytes, so a
// plugin can never send a credential, or anything else, before the channel is encrypted.
const (
	tcpModeTLS          = "tls"
	tcpModeSMTPStartTLS = "smtp-starttls"
	tcpModeIMAPStartTLS = "imap-starttls"
)

type hostTCPConn struct {
	conn     net.Conn
	host     string
	port     int
	sent     atomic.Int64
	received atomic.Int64
}

func (h *HostContext) byteLimit() int64 {
	if h.TCPByteLimit > 0 {
		return h.TCPByteLimit
	}
	return defaultTCPBytes
}

// CloseAllTCP closes every connection the plugin still holds (called when it is unloaded).
func (h *HostContext) CloseAllTCP() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, c := range h.tcpConns {
		_ = c.conn.Close()
	}
	h.tcpConns = nil
}

func (h *HostContext) tcpConn(id int32) *hostTCPConn {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.tcpConns[id]
}

type tcpConnectRequest struct {
	Host string `json:"host"`
	Port int    `json:"port"`
	Mode string `json:"mode"`
}

// tcpConnect implements acton_tcp.connect(reqPtr, reqLen) -> handle | negative code.
func tcpConnect(ctx context.Context, m api.Module, ptr, length uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil || h.Gate == nil {
		return TCPErrNoHost
	}
	raw, err := readBufferFromMemory(m.Memory(), ptr, length)
	if err != nil || len(raw) > 1024 {
		return TCPErrDenied
	}
	var req tcpConnectRequest
	if json.Unmarshal(raw, &req) != nil {
		return TCPErrDenied
	}
	if req.Mode == "" {
		req.Mode = tcpModeTLS
	}
	switch req.Mode {
	case tcpModeTLS, tcpModeSMTPStartTLS, tcpModeIMAPStartTLS:
	default:
		return TCPErrDenied
	}
	if err := h.Gate.CheckTCPAccess(req.Host, req.Port); err != nil {
		h.Record("WARN", err.Error())
		return TCPErrDenied
	}
	h.mu.Lock()
	if len(h.tcpConns) >= maxTCPConns {
		h.mu.Unlock()
		return TCPErrTooMany
	}
	h.mu.Unlock()

	conn, err := dialMode(ctx, h.Dial, req)
	if err != nil {
		h.Record("WARN", fmt.Sprintf("tcp connect %s:%d failed: %v", req.Host, req.Port, err))
		if errors.Is(err, security.ErrUnsafeAddress) {
			return TCPErrBlocked
		}
		return TCPErrConnect
	}

	h.mu.Lock()
	if len(h.tcpConns) >= maxTCPConns { // another call won the race for the last slot
		h.mu.Unlock()
		_ = conn.Close()
		return TCPErrTooMany
	}
	if h.tcpConns == nil {
		h.tcpConns = map[int32]*hostTCPConn{}
	}
	h.nextTCPID++
	id := h.nextTCPID
	h.tcpConns[id] = &hostTCPConn{conn: conn, host: req.Host, port: req.Port}
	h.mu.Unlock()
	h.Record("INFO", fmt.Sprintf("tcp connected %s:%d (%s)", req.Host, req.Port, req.Mode)) // never the content
	return id
}

func dialMode(ctx context.Context, p security.DialPolicy, req tcpConnectRequest) (net.Conn, error) {
	if req.Mode == tcpModeTLS {
		return p.DialTLS(ctx, req.Host, req.Port)
	}
	conn, err := p.DialTCP(ctx, req.Host, req.Port)
	if err != nil {
		return nil, err
	}
	_ = conn.SetDeadline(time.Now().Add(tcpPreambleTimeout))
	var perr error
	if req.Mode == tcpModeSMTPStartTLS {
		perr = smtpStartTLS(conn)
	} else {
		perr = imapStartTLS(conn)
	}
	if perr != nil {
		_ = conn.Close()
		return nil, perr
	}
	_ = conn.SetDeadline(time.Time{})
	return p.UpgradeTLS(ctx, conn, req.Host)
}

// readReply reads one SMTP reply (possibly multi-line) and returns its lines.
func readSMTPReply(br *bufio.Reader) (code string, lines []string, err error) {
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			return "", nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if len(line) < 4 {
			return "", nil, fmt.Errorf("malformed SMTP reply %q", line)
		}
		lines = append(lines, line)
		if line[3] == ' ' {
			return line[:3], lines, nil
		}
		if line[3] != '-' || len(lines) > 100 {
			return "", nil, fmt.Errorf("malformed SMTP reply %q", line)
		}
	}
}

func smtpStartTLS(conn net.Conn) error {
	br := bufio.NewReader(io.LimitReader(conn, 64<<10))
	if code, _, err := readSMTPReply(br); err != nil || code != "220" {
		return fmt.Errorf("the SMTP server did not greet us (%q): %v", code, err)
	}
	if _, err := conn.Write([]byte("EHLO acton.invalid\r\n")); err != nil {
		return err
	}
	code, lines, err := readSMTPReply(br)
	if err != nil || code != "250" {
		return fmt.Errorf("the SMTP server rejected EHLO (%q): %v", code, err)
	}
	offered := false
	for _, l := range lines {
		if strings.EqualFold(strings.TrimSpace(l[4:]), "STARTTLS") {
			offered = true
		}
	}
	if !offered {
		return errors.New("the SMTP server does not offer STARTTLS; refusing to continue without encryption")
	}
	if _, err := conn.Write([]byte("STARTTLS\r\n")); err != nil {
		return err
	}
	if code, _, err := readSMTPReply(br); err != nil || code != "220" {
		return fmt.Errorf("the SMTP server refused STARTTLS (%q): %v", code, err)
	}
	if br.Buffered() != 0 {
		// Bytes that arrive before the handshake could have been injected by an attacker in the
		// middle; they would be read as if they were part of the encrypted session.
		return errors.New("unexpected data after STARTTLS; aborting")
	}
	return nil
}

func imapStartTLS(conn net.Conn) error {
	br := bufio.NewReader(io.LimitReader(conn, 64<<10))
	greeting, err := br.ReadString('\n')
	if err != nil || !strings.HasPrefix(greeting, "* OK") {
		return fmt.Errorf("the IMAP server did not greet us with OK: %v", err)
	}
	if _, err := conn.Write([]byte("a1 STARTTLS\r\n")); err != nil {
		return err
	}
	for i := 0; i < 100; i++ {
		line, err := br.ReadString('\n')
		if err != nil {
			return err
		}
		switch {
		case strings.HasPrefix(line, "a1 OK"):
			if br.Buffered() != 0 {
				return errors.New("unexpected data after STARTTLS; aborting")
			}
			return nil
		case strings.HasPrefix(line, "a1 "):
			return errors.New("the IMAP server refused STARTTLS")
		}
	}
	return errors.New("the IMAP server sent too many lines before answering STARTTLS")
}

// tcpSend implements acton_tcp.send(handle, ptr, len) -> bytes written | negative code.
func tcpSend(ctx context.Context, m api.Module, handle int32, ptr, length uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil {
		return TCPErrNoHost
	}
	c := h.tcpConn(handle)
	if c == nil {
		return TCPErrClosed
	}
	if length == 0 || length > maxTCPSend {
		return TCPErrDenied
	}
	if c.sent.Load()+int64(length) > h.byteLimit() {
		return TCPErrOverLimit
	}
	data, err := readBufferFromMemory(m.Memory(), ptr, length)
	if err != nil {
		return TCPErrDenied
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(tcpIdle))
	n, err := c.conn.Write(data)
	c.sent.Add(int64(n))
	if err != nil {
		return TCPErrConnect
	}
	return int32(n)
}

// tcpRecv implements acton_tcp.recv(handle, max, timeoutMs) -> byte count (0 = timeout, data
// is in the response buffer for acton_sys.read_response) | negative code.
func tcpRecv(ctx context.Context, m api.Module, handle int32, max uint32, timeoutMS uint32) int32 {
	h := HostContextFrom(ctx)
	if h == nil {
		return TCPErrNoHost
	}
	c := h.tcpConn(handle)
	if c == nil {
		return TCPErrClosed
	}
	if max == 0 || max > maxTCPRecv {
		max = maxTCPRecv
	}
	wait := time.Duration(timeoutMS) * time.Millisecond
	if wait <= 0 || wait > tcpIdle {
		wait = tcpIdle
	}
	if c.received.Load() >= h.byteLimit() {
		return TCPErrOverLimit
	}
	buf := make([]byte, max)
	_ = c.conn.SetReadDeadline(time.Now().Add(wait))
	n, err := c.conn.Read(buf)
	c.received.Add(int64(n))
	if n > 0 {
		h.mu.Lock()
		h.LastResponse = append([]byte(nil), buf[:n]...)
		h.mu.Unlock()
		return int32(n)
	}
	var ne net.Error
	if errors.As(err, &ne) && ne.Timeout() {
		return 0
	}
	h.Record("DEBUG", fmt.Sprintf("tcp recv ended: %v", err))
	return TCPErrClosed
}

// tcpClose implements acton_tcp.close(handle).
func tcpClose(ctx context.Context, _ api.Module, handle int32) int32 {
	h := HostContextFrom(ctx)
	if h == nil {
		return TCPErrNoHost
	}
	h.mu.Lock()
	c := h.tcpConns[handle]
	delete(h.tcpConns, handle)
	h.mu.Unlock()
	if c == nil {
		return TCPErrClosed
	}
	_ = c.conn.Close()
	return 0
}

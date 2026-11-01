//go:build wasip1

package wasmguest

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"runtime"
	"time"
)

//go:wasmimport acton_tcp connect
func hostTCPConnect(ptr, length uint32) int32

//go:wasmimport acton_tcp send
func hostTCPSend(handle int32, ptr, length uint32) int32

//go:wasmimport acton_tcp recv
func hostTCPRecv(handle int32, max, timeoutMS uint32) int32

//go:wasmimport acton_tcp close
func hostTCPClose(handle int32) int32

// Modes understood by the host. There is no plaintext mode: STARTTLS is run by the host.
const (
	ModeTLS          = "tls"
	ModeSMTPStartTLS = "smtp-starttls"
	ModeIMAPStartTLS = "imap-starttls"
)

// TCPError carries the host's refusal code.
type TCPError struct {
	Op   string
	Code int32
}

func (e *TCPError) Error() string {
	reason := map[int32]string{-1: "no host", -2: "not permitted by the plugin manifest", -3: "the address is local or private",
		-4: "connection or TLS failed", -5: "too many connections", -6: "traffic limit reached", -7: "connection closed"}[e.Code]
	if reason == "" {
		reason = fmt.Sprintf("code %d", e.Code)
	}
	return fmt.Sprintf("tcp %s: %s", e.Op, reason)
}

// pollSlice is how long one host recv may block. The guest is single-threaded: a longer block
// would stop every other goroutine (for example the one that writes the next command).
const pollSlice = 20

// Conn is a net.Conn over a host TCP handle. Reads poll the host in short slices and yield to
// other goroutines between slices, which is what makes libraries that read and write from
// separate goroutines (go-imap) work inside WebAssembly.
type Conn struct {
	handle int32
	rdl    time.Time
	wdl    time.Time
	closed bool
	buf    []byte
}

var _ net.Conn = (*Conn)(nil)

// DialTCP asks the host for a connection to host:port in the given mode.
func DialTCP(host string, port int, mode string) (*Conn, error) {
	req, _ := json.Marshal(map[string]any{"host": host, "port": port, "mode": mode})
	h := hostTCPConnect(ptr(req), uint32(len(req)))
	if h <= 0 {
		return nil, &TCPError{Op: "connect", Code: h}
	}
	c := &Conn{handle: h}
	if mode == ModeSMTPStartTLS {
		// The host consumed the server's greeting while it ran STARTTLS. Stock SMTP clients wait for
		// one before they send EHLO, so present a synthetic greeting; the server's real next words
		// are its answer to that EHLO.
		c.buf = []byte("220 acton: the channel is now encrypted\r\n")
	}
	return c, nil
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

// Read implements net.Conn.
func (c *Conn) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	for {
		if len(c.buf) > 0 {
			n := copy(p, c.buf)
			c.buf = c.buf[n:]
			return n, nil
		}
		if c.closed {
			return 0, net.ErrClosed
		}
		if !c.rdl.IsZero() && !time.Now().Before(c.rdl) {
			return 0, timeoutError{}
		}
		n := hostTCPRecv(c.handle, uint32(len(p)), pollSlice)
		switch {
		case n > 0:
			data, err := readHostResponse(n)
			if err != nil {
				return 0, err
			}
			c.buf = data
		case n == 0:
			runtime.Gosched()
			time.Sleep(time.Millisecond)
		case n == -7:
			return 0, io.EOF
		default:
			return 0, &TCPError{Op: "recv", Code: n}
		}
	}
}

// Write implements net.Conn.
func (c *Conn) Write(p []byte) (int, error) {
	if c.closed {
		return 0, net.ErrClosed
	}
	written := 0
	for written < len(p) {
		if !c.wdl.IsZero() && !time.Now().Before(c.wdl) {
			return written, timeoutError{}
		}
		chunk := p[written:]
		if len(chunk) > 256<<10 {
			chunk = chunk[:256<<10]
		}
		n := hostTCPSend(c.handle, ptr(chunk), uint32(len(chunk)))
		if n < 0 {
			return written, &TCPError{Op: "send", Code: n}
		}
		written += int(n)
	}
	return written, nil
}

// Close implements net.Conn.
func (c *Conn) Close() error {
	if c.closed {
		return nil
	}
	c.closed = true
	if hostTCPClose(c.handle) < 0 {
		return errors.New("tcp close failed")
	}
	return nil
}

type addr string

func (a addr) Network() string { return "tcp" }
func (a addr) String() string  { return string(a) }

func (c *Conn) LocalAddr() net.Addr  { return addr("guest") }
func (c *Conn) RemoteAddr() net.Addr { return addr(fmt.Sprintf("host-handle-%d", c.handle)) }

func (c *Conn) SetDeadline(t time.Time) error      { c.rdl, c.wdl = t, t; return nil }
func (c *Conn) SetReadDeadline(t time.Time) error  { c.rdl = t; return nil }
func (c *Conn) SetWriteDeadline(t time.Time) error { c.wdl = t; return nil }

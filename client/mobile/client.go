// Package mobile is a gomobile-bind friendly facade over the vinereal client
// SDK.
//
// gomobile cannot bind net.Conn directly: its SetDeadline takes time.Time,
// and interfaces/generics don't cross the binding boundary. This package wraps
// the already-REALITY-authenticated connection in a small struct whose
// methods use only bindable types (signed/unsigned integers, float, string,
// []byte and error), matching the exact shape gomobile's bind generator
// understands.
//
// Build with:
//
//	gomobile bind -target=android -o vinereal.aar ./mobile
//	gomobile bind -target=ios    -o VineReal.xcframework ./mobile
//
// from inside client/ (its go.mod carries the replace to ../shared, so the
// resulting dependency tree stays utls-only; the root go.work is irrelevant to
// gomobile).
package mobile

import (
	"errors"
	"net"
	"time"

	"github.com/VineLink-Lab/VineReal/client/vinereal"
)

// Conn wraps a vinereal connection. Read/Write/Close/SetDeadline are the only
// operations a mobile caller needs; the REALITY handshake and the vision
// padding exchange already completed before this wrapper is handed back.
type Conn struct {
	inner net.Conn
}

func (c *Conn) Read(b []byte) (int, error) {
	if c == nil || c.inner == nil {
		return 0, net.ErrClosed
	}
	return c.inner.Read(b)
}

func (c *Conn) Write(b []byte) (int, error) {
	if c == nil || c.inner == nil {
		return 0, net.ErrClosed
	}
	return c.inner.Write(b)
}

func (c *Conn) Close() error {
	if c == nil || c.inner == nil {
		return nil
	}
	return c.inner.Close()
}

// SetDeadlineMillis sets both the read and write deadlines to the given Unix
// millisecond timestamp. Pass 0 to clear the deadline.
func (c *Conn) SetDeadlineMillis(unixMs int64) error {
	if c == nil || c.inner == nil {
		return net.ErrClosed
	}
	return c.inner.SetDeadline(millis(unixMs))
}

// SetReadDeadlineMillis sets only the read deadline. Pass 0 to clear it.
func (c *Conn) SetReadDeadlineMillis(unixMs int64) error {
	if c == nil || c.inner == nil {
		return net.ErrClosed
	}
	return c.inner.SetReadDeadline(millis(unixMs))
}

// SetWriteDeadlineMillis sets only the write deadline. Pass 0 to clear it.
func (c *Conn) SetWriteDeadlineMillis(unixMs int64) error {
	if c == nil || c.inner == nil {
		return net.ErrClosed
	}
	return c.inner.SetWriteDeadline(millis(unixMs))
}

func millis(ms int64) time.Time {
	if ms <= 0 {
		return time.Time{}
	}
	return time.UnixMilli(ms)
}

// Client wraps a vinereal.Client for gomobile.
type Client struct {
	inner *vinereal.Client
}

// NewClient builds a REALITY client from bindable scalars.
//
// fingerprint may be empty, in which case "chrome_auto" is used.
// handshakeTimeoutMs bounds the whole dial + handshake + vision-exchange
// sequence; a value <= 0 falls back to the SDK default (10s).
//
// It returns an error for obviously incomplete identity material so a bad
// deployment config fails at construction time rather than at dial time.
func NewClient(serverAddr, publicKeyB64, shortIDHex, serverName, fingerprint string, handshakeTimeoutMs int64) (*Client, error) {
	switch {
	case serverAddr == "":
		return nil, errors.New("mobile: serverAddr is required")
	case publicKeyB64 == "":
		return nil, errors.New("mobile: publicKeyB64 is required")
	case serverName == "":
		return nil, errors.New("mobile: serverName is required")
	}

	cfg := vinereal.DefaultConfig
	cfg.ServerAddr = serverAddr
	cfg.PublicKeyB64 = publicKeyB64
	cfg.ShortIDHex = shortIDHex
	cfg.ServerName = serverName
	if fingerprint != "" {
		cfg.Fingerprint = fingerprint
	}
	if handshakeTimeoutMs > 0 {
		cfg.HandshakeTimeoutMS = handshakeTimeoutMs
	}

	return &Client{inner: vinereal.NewClient(cfg)}, nil
}

// Dial establishes a REALITY-authenticated, traffic-shaped connection and
// returns it ready for application use. network must be "tcp"; addr is
// accepted only for net.Dial-shaped call-site compatibility and is ignored
// (the real destination is fixed by the server operator's upstream config).
func (c *Client) Dial(network, addr string) (*Conn, error) {
	if c == nil || c.inner == nil {
		return nil, errors.New("mobile: nil client")
	}
	conn, err := c.inner.Dial(network, addr)
	if err != nil {
		return nil, err
	}
	return &Conn{inner: conn}, nil
}

// Package proxy builds the REALITY listener and runs the per-connection
// reverse-proxy loop that forwards authenticated traffic to the configured
// fixed upstream.
package proxy

import (
	"net"

	"github.com/VineLink-Lab/VineReal/server/config"
	"github.com/xtls/reality"
)

// NewListener starts a plain TCP listener on cfg.Listen and wraps it with
// reality.NewListener. Every net.Conn later returned by the wrapped
// listener's Accept is already a fully REALITY-authenticated connection:
// xtls/reality handles unauthenticated/probing connections internally by
// splicing them through to cfg.Dest and never surfaces them here.
//
// Deliberately do not wrap ln in any decorator before handing it to
// reality.NewListener: its accept loop does a bare type assertion to a
// CloseWrite-capable connection internally, which panics on anything that
// isn't (effectively) a *net.TCPConn.
func NewListener(cfg *config.Config) (net.Listener, error) {
	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return nil, err
	}

	var dialer net.Dialer
	rc := &reality.Config{
		DialContext:  dialer.DialContext,
		Show:         cfg.Debug,
		Type:         "tcp",
		Dest:         cfg.Dest,
		ServerNames:  cfg.ServerNames,
		PrivateKey:   cfg.PrivateKey,
		MinClientVer: cfg.MinClientVer,
		MaxClientVer: cfg.MaxClientVer,
		MaxTimeDiff:  cfg.MaxTimeDiff,
		ShortIds:     cfg.ShortIDs,

		// xtls/reality ships two competing NewSessionTicket paths: the
		// standard crypto/tls sendSessionTickets (a real, resumable ticket)
		// and the REALITY record-length mimicry in encrypt(). When both run,
		// encrypt()'s typeNewSessionTicket branch corrupts the first byte of
		// the real ticket (0x04 -> 0x17), producing an invalid post-handshake
		// handshake record that clients reject with an unexpected_message
		// alert. Disable the real ticket so only the mimicry runs; the
		// padding/mimicry already reproduces the decoy's ticket length, so no
		// anti-fingerprinting behavior is lost.
		SessionTicketsDisabled: true,
	}

	return reality.NewListener(ln, rc), nil
}

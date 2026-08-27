package proxy

import (
	"crypto/tls"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/VineLink-Lab/VineReal/server/config"
	"github.com/VineLink-Lab/VineReal/shared/visionframe"
)

// Serve runs the accept loop on ln, reverse-proxying every connection to
// cfg.Upstream. It blocks until Accept returns a permanent error (typically
// because ln was closed for shutdown), which it then returns to the caller.
func Serve(ln net.Listener, cfg *config.Config) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go handle(conn, cfg)
	}
}

func handle(conn net.Conn, cfg *config.Config) {
	defer conn.Close()
	remote := conn.RemoteAddr()

	if err := visionframe.Exchange(conn, cfg.Vision); err != nil {
		slog.Warn("vision exchange failed, dropping connection", "remote", remote, "err", err)
		return
	}

	upstream, err := dialUpstream(cfg)
	if err != nil {
		slog.Warn("failed to dial upstream", "remote", remote, "upstream", cfg.Upstream, "err", err)
		return
	}
	defer upstream.Close()

	slog.Info("proxying connection", "remote", remote, "upstream", cfg.Upstream)
	relay(conn, upstream)
	slog.Info("connection closed", "remote", remote)
}

func dialUpstream(cfg *config.Config) (net.Conn, error) {
	if cfg.UpstreamTLS {
		return tls.Dial("tcp", cfg.Upstream, &tls.Config{ServerName: upstreamServerName(cfg.Upstream)})
	}
	return net.Dial("tcp", cfg.Upstream)
}

func upstreamServerName(addr string) string {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return addr
	}
	return host
}

// relay pumps bytes in both directions between a and b until one side is
// done, then closes both ends. It waits for both io.Copy goroutines to
// return before giving control back to the caller, so the caller's deferred
// Close calls don't race with in-flight copies.
func relay(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(b, a) //nolint:errcheck // best-effort relay, connection errors are expected on close
		closeWrite(b)
	}()
	go func() {
		defer wg.Done()
		io.Copy(a, b) //nolint:errcheck
		closeWrite(a)
	}()

	wg.Wait()
}

type closeWriter interface {
	CloseWrite() error
}

// closeWrite half-closes c's write side if it supports it (both
// *reality.Conn and *net.TCPConn do), letting the peer see EOF while the
// other direction keeps draining. Falls back to a full Close otherwise.
func closeWrite(c net.Conn) {
	if cw, ok := c.(closeWriter); ok {
		cw.CloseWrite()
		return
	}
	c.Close()
}

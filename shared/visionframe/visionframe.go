// Package visionframe implements a small, symmetric, stdlib-only padding
// exchange that both the REALITY client and server run immediately after the
// TLS/REALITY handshake completes and before raw byte passthrough begins.
//
// The goal is not to hide protocol content (that's already done by TLS) but
// to blur the packet-size/timing signature of "handshake just finished, now
// a proxy session starts" by emitting a few randomly-sized frames first, the
// way a real browser would fetch several page resources right after loading
// a site. Each side randomizes independently within its own configured
// bounds; there is no negotiation, the exchange is self-delimiting via a
// terminator frame.
package visionframe

import (
	"encoding/binary"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"sync"
	"time"
)

const (
	frameHeaderLen = 3 // 1 byte type + 2 byte big-endian length

	typePadding    byte = 0x01
	typeTerminator byte = 0x02
)

// Config controls one side's padding behavior. The two sides do not need
// matching configs.
type Config struct {
	MinFrames, MaxFrames             int
	MinPaddingBytes, MaxPaddingBytes int
	MinDelayMS, MaxDelayMS           int
}

// DefaultConfig is a reasonable default mimicking a handful of small
// resource fetches after a page load.
var DefaultConfig = Config{
	MinFrames:       2,
	MaxFrames:       6,
	MinPaddingBytes: 64,
	MaxPaddingBytes: 1440,
	MinDelayMS:      0,
	MaxDelayMS:      30,
}

// Exchange runs the padding handshake over conn: it emits a randomized burst
// of padding frames followed by a terminator, while concurrently draining
// the peer's own padding frames until it sees their terminator. It returns
// once both directions are done, at which point conn is positioned exactly
// at the first byte of real application data on both ends and is safe to
// hand off to a plain io.Copy-based relay.
func Exchange(conn net.Conn, cfg Config) error {
	var wg sync.WaitGroup
	wg.Add(2)

	var writeErr, readErr error
	go func() {
		defer wg.Done()
		writeErr = writeLoop(conn, cfg)
	}()
	go func() {
		defer wg.Done()
		readErr = readLoop(conn)
	}()
	wg.Wait()

	if writeErr != nil {
		return fmt.Errorf("visionframe: write: %w", writeErr)
	}
	if readErr != nil {
		return fmt.Errorf("visionframe: read: %w", readErr)
	}
	return nil
}

func writeLoop(w io.Writer, cfg Config) error {
	n := randRange(cfg.MinFrames, cfg.MaxFrames)
	for i := 0; i < n; i++ {
		length := randRange(cfg.MinPaddingBytes, cfg.MaxPaddingBytes)
		if err := writeFrame(w, typePadding, make([]byte, length)); err != nil {
			return err
		}
		if cfg.MaxDelayMS > 0 {
			if d := randRange(cfg.MinDelayMS, cfg.MaxDelayMS); d > 0 {
				time.Sleep(time.Duration(d) * time.Millisecond)
			}
		}
	}
	return writeFrame(w, typeTerminator, nil)
}

func readLoop(r io.Reader) error {
	header := make([]byte, frameHeaderLen)
	for {
		if _, err := io.ReadFull(r, header); err != nil {
			return err
		}
		typ := header[0]
		length := binary.BigEndian.Uint16(header[1:3])

		switch typ {
		case typeTerminator:
			if length != 0 {
				return fmt.Errorf("terminator frame with nonzero length %d", length)
			}
			return nil
		case typePadding:
			if length > 0 {
				if _, err := io.CopyN(io.Discard, r, int64(length)); err != nil {
					return err
				}
			}
		default:
			return fmt.Errorf("unknown frame type 0x%02x", typ)
		}
	}
}

func writeFrame(w io.Writer, typ byte, payload []byte) error {
	buf := make([]byte, frameHeaderLen+len(payload))
	buf[0] = typ
	binary.BigEndian.PutUint16(buf[1:3], uint16(len(payload)))
	copy(buf[frameHeaderLen:], payload)
	_, err := w.Write(buf)
	return err
}

// randRange returns a random int in [lo, hi] inclusive. If hi <= lo it
// returns lo, so callers can pass degenerate bounds (e.g. both 0) safely.
func randRange(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + rand.IntN(hi-lo+1)
}

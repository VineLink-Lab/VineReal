package vinereal

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	utls "github.com/refraction-networking/utls"

	"github.com/VineLink-Lab/VineReal/shared/visionframe"
)

// Client dials a single REALITY-fronted reverse-proxy deployment described
// by Config.
type Client struct {
	cfg Config
}

// NewClient returns a Client for cfg. cfg is copied; mutating it afterwards
// has no effect on the returned Client.
func NewClient(cfg Config) *Client {
	return &Client{cfg: cfg}
}

// Dial establishes a REALITY-authenticated, traffic-shaped connection and
// returns it as a plain net.Conn ready for application use.
//
// network must be "tcp" (REALITY runs over TCP). addr is accepted only for
// call-site compatibility with net.Dial-shaped signatures (e.g. plugging
// this into http.Transport.DialContext) and is otherwise ignored: per this
// project's fixed-backend design, the real destination is entirely
// determined by the server operator's own upstream config, not by the
// caller. This is not a general-purpose SOCKS-style dialer.
func (c *Client) Dial(network, addr string) (net.Conn, error) {
	return c.DialContext(context.Background(), network, addr)
}

// DialContext is Dial with caller-supplied cancellation, in addition to the
// client's own configured handshake timeout.
func (c *Client) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	if network != "tcp" {
		return nil, fmt.Errorf("vinereal: unsupported network %q, only \"tcp\" is supported", network)
	}
	_ = addr // intentionally ignored, see Dial's doc comment

	timeout := time.Duration(c.cfg.HandshakeTimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	dialCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var d net.Dialer
	raw, err := d.DialContext(dialCtx, "tcp", c.cfg.ServerAddr)
	if err != nil {
		return nil, fmt.Errorf("vinereal: dial %s: %w", c.cfg.ServerAddr, err)
	}

	fingerprint, err := lookupFingerprint(c.cfg.Fingerprint)
	if err != nil {
		raw.Close()
		return nil, err
	}

	auth := &authenticator{}
	utlsCfg := &utls.Config{
		ServerName:             c.cfg.ServerName,
		InsecureSkipVerify:     true,
		SessionTicketsDisabled: true,
		VerifyPeerCertificate:  auth.verifyPeerCertificate,
	}
	uconn := utls.UClient(raw, utlsCfg, fingerprint)

	if err := auth.encode(uconn, c.cfg); err != nil {
		raw.Close()
		return nil, err
	}

	if err := uconn.HandshakeContext(dialCtx); err != nil {
		raw.Close()
		return nil, fmt.Errorf("vinereal: handshake: %w", err)
	}
	if !auth.verified {
		// Belt-and-suspenders: verifyPeerCertificate returning nil is what
		// actually gates handshake success, so this should be unreachable,
		// but never hand back a connection we haven't positively verified.
		uconn.Close()
		return nil, errors.New("vinereal: handshake succeeded but server identity was not verified")
	}

	if err := visionframe.Exchange(uconn, c.cfg.Vision); err != nil {
		uconn.Close()
		return nil, fmt.Errorf("vinereal: vision exchange: %w", err)
	}

	return uconn, nil
}

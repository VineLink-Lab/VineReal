// Package e2e drives a real vinereal-server and a real vinereal client
// against each other over actual TCP sockets on localhost, proving the
// REALITY handshake, the vision padding exchange and the fixed-backend
// reverse proxy all work together — and, just as importantly, that
// unauthenticated connections are genuinely and transparently spliced
// through to the decoy site rather than erroring out or reaching the real
// backend.
package e2e

import (
	"bufio"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"io"
	"math/big"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/VineLink-Lab/VineReal/client/vinereal"
	"github.com/VineLink-Lab/VineReal/server/config"
	"github.com/VineLink-Lab/VineReal/server/proxy"
	"github.com/VineLink-Lab/VineReal/shared/visionframe"
)

const (
	decoyBody    = "hello from the decoy site"
	upstreamBody = "hello from the real upstream backend"
	testSNI      = "localhost"
	validShortID = "aabbccddeeff0011"
)

func TestPositiveEndToEnd(t *testing.T) {
	privKey, pubKey := genX25519KeyPair(t)
	shortIDs := map[[8]byte]bool{decodeHexShortID(t, validShortID): true}

	upstreamAddr := startUpstream(t)
	decoyAddr := startDecoy(t)
	serverAddr := startRealityServer(t, upstreamAddr, decoyAddr, privKey, shortIDs)

	client := vinereal.NewClient(vinereal.Config{
		ServerAddr:         serverAddr,
		ServerName:         testSNI,
		PublicKeyB64:       base64.RawURLEncoding.EncodeToString(pubKey),
		ShortIDHex:         validShortID,
		Fingerprint:        "chrome_auto",
		HandshakeTimeoutMS: 5000,
		Vision:             visionframe.Config{MinFrames: 1, MaxFrames: 2, MinPaddingBytes: 1, MaxPaddingBytes: 8},
	})

	conn, err := client.Dial("tcp", "ignored:0")
	if err != nil {
		t.Fatalf("client Dial: %v", err)
	}
	defer conn.Close()

	body := doHTTPGet(t, conn, testSNI)
	if !strings.Contains(body, upstreamBody) {
		t.Fatalf("expected upstream body, got: %q", body)
	}
}

func TestClientRejectsUnrecognizedShortID(t *testing.T) {
	privKey, pubKey := genX25519KeyPair(t)
	shortIDs := map[[8]byte]bool{decodeHexShortID(t, validShortID): true}

	upstreamAddr := startUpstream(t)
	decoyAddr := startDecoy(t)
	serverAddr := startRealityServer(t, upstreamAddr, decoyAddr, privKey, shortIDs)

	client := vinereal.NewClient(vinereal.Config{
		ServerAddr:         serverAddr,
		ServerName:         testSNI,
		PublicKeyB64:       base64.RawURLEncoding.EncodeToString(pubKey),
		ShortIDHex:         "0000000000000000", // not in the server's allowed set
		Fingerprint:        "chrome_auto",
		HandshakeTimeoutMS: 5000,
	})

	if _, err := client.Dial("tcp", "ignored:0"); err == nil {
		t.Fatal("expected Dial to fail for an unrecognized short id, got a nil error")
	} else {
		t.Logf("got expected rejection: %v", err)
	}
}

// TestUnauthenticatedConnectionFallsBackToDecoy bypasses the vinereal client
// entirely and connects with a plain stdlib crypto/tls.Dial: no REALITY auth
// token at all. This is the server-side ground truth that xtls/reality's
// fallback behavior actually works — the connection should transparently
// land on the decoy site's real TLS session and content, never on our
// upstream, and never as a bare connection error.
func TestUnauthenticatedConnectionFallsBackToDecoy(t *testing.T) {
	privKey, _ := genX25519KeyPair(t)
	shortIDs := map[[8]byte]bool{decodeHexShortID(t, validShortID): true}

	upstreamAddr := startUpstream(t)
	decoyAddr := startDecoy(t)
	serverAddr := startRealityServer(t, upstreamAddr, decoyAddr, privKey, shortIDs)

	rawConn, err := tls.Dial("tcp", serverAddr, &tls.Config{
		ServerName:         testSNI,
		InsecureSkipVerify: true, // decoy uses a self-signed test cert
	})
	if err != nil {
		t.Fatalf("bare tls.Dial: %v", err)
	}
	defer rawConn.Close()

	body := doHTTPGet(t, rawConn, testSNI)
	if strings.Contains(body, upstreamBody) {
		t.Fatalf("unauthenticated connection reached the real upstream backend! got: %q", body)
	}
	if !strings.Contains(body, decoyBody) {
		t.Fatalf("expected decoy body for an unauthenticated connection, got: %q", body)
	}
}

// --- test fixtures ---

func startUpstream(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen upstream: %v", err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, upstreamBody)
	})}
	go srv.Serve(ln)
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().String()
}

func startDecoy(t *testing.T) string {
	t.Helper()
	cert := generateSelfSignedCert(t, testSNI)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen decoy: %v", err)
	}
	tlsLn := tls.NewListener(ln, &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
	})
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		io.WriteString(w, decoyBody)
	})}
	go srv.Serve(tlsLn)
	t.Cleanup(func() { srv.Close() })
	return ln.Addr().String()
}

func startRealityServer(t *testing.T, upstreamAddr, decoyAddr string, privKey []byte, shortIDs map[[8]byte]bool) string {
	t.Helper()
	cfg := &config.Config{
		Listen:      "127.0.0.1:0",
		Upstream:    upstreamAddr,
		Dest:        decoyAddr,
		ServerNames: map[string]bool{testSNI: true},
		PrivateKey:  privKey,
		ShortIDs:    shortIDs,
		Vision:      visionframe.Config{MinFrames: 1, MaxFrames: 2, MinPaddingBytes: 1, MaxPaddingBytes: 8},
		Debug:       true,
	}
	ln, err := proxy.NewListener(cfg)
	if err != nil {
		t.Fatalf("start reality listener: %v", err)
	}
	go proxy.Serve(ln, cfg)
	t.Cleanup(func() { ln.Close() })
	return ln.Addr().String()
}

func genX25519KeyPair(t *testing.T) (priv, pub []byte) {
	t.Helper()
	k, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate x25519 key: %v", err)
	}
	return k.Bytes(), k.PublicKey().Bytes()
}

func decodeHexShortID(t *testing.T, s string) [8]byte {
	t.Helper()
	b, err := hex.DecodeString(s)
	if err != nil {
		t.Fatalf("decode short id %q: %v", s, err)
	}
	var out [8]byte
	copy(out[:], b)
	return out
}

func generateSelfSignedCert(t *testing.T, dnsName string) tls.Certificate {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate decoy key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: dnsName},
		DNSNames:     []string{dnsName},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("create decoy cert: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: priv}
}

func doHTTPGet(t *testing.T, conn net.Conn, host string) string {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, "/", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Host = host
	req.Close = true

	conn.SetDeadline(time.Now().Add(10 * time.Second))
	if err := req.Write(conn); err != nil {
		t.Fatalf("write request: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(body)
}

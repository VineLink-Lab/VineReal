package vinereal

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ed25519"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	utls "github.com/refraction-networking/utls"
)

// sdkVersion is embedded in every ClientHello's auth payload. A REALITY
// server may optionally enforce reality.Config.MinClientVer/MaxClientVer
// against it; vinereal-server's example config leaves that check disabled.
var sdkVersion = [3]byte{1, 0, 0}

// authenticator carries the per-connection state needed both to embed the
// REALITY auth token into the outgoing ClientHello and, later, to verify
// that the server's self-signed certificate really was minted by the
// holder of the matching REALITY private key.
type authenticator struct {
	authKey  []byte // 32-byte AES-256 key derived via ECDH+HKDF, filled by encode
	verified bool
}

// encode mutates uconn's about-to-be-sent ClientHello in place, embedding
// the REALITY auth token (version + timestamp + short id, AEAD-sealed)
// into the session id field. Must be called after utls.UClient and before
// HandshakeContext.
//
// The byte layout and crypto here are dictated by the REALITY protocol, as
// implemented server-side by github.com/xtls/reality's TLS 1.3 handshake:
// this is a wire-format interop requirement the client must match exactly,
// not a design choice made independently here.
func (a *authenticator) encode(uconn *utls.UConn, cfg Config) error {
	if err := uconn.BuildHandshakeState(); err != nil {
		return fmt.Errorf("vinereal: build handshake state: %w", err)
	}

	hello := uconn.HandshakeState.Hello
	const sessionIDOffset = 39 // 1 (msg type) + 3 (len) + 2 (client version) + 32 (random) + 1 (sessionid len byte)
	if len(hello.Raw) < sessionIDOffset+32 {
		return errors.New("vinereal: ClientHello too short to carry a 32-byte session id")
	}
	if len(hello.Random) < 32 {
		return errors.New("vinereal: ClientHello random is shorter than 32 bytes")
	}

	// Zero the session id both in the struct field and in the raw wire
	// bytes: Raw, with a zeroed session id, is used below as AEAD
	// associated data, and the server reconstructs the identical zeroed
	// state before calling Open.
	hello.SessionId = make([]byte, 32)
	copy(hello.Raw[sessionIDOffset:], hello.SessionId)

	shortID, err := decodeShortID(cfg.ShortIDHex)
	if err != nil {
		return fmt.Errorf("vinereal: short id: %w", err)
	}

	// Build the 16-byte auth payload directly into hello.SessionId; it is
	// AEAD-sealed in place below.
	hello.SessionId[0] = sdkVersion[0]
	hello.SessionId[1] = sdkVersion[1]
	hello.SessionId[2] = sdkVersion[2]
	hello.SessionId[3] = 0 // reserved
	binary.BigEndian.PutUint32(hello.SessionId[4:8], uint32(time.Now().Unix()))
	copy(hello.SessionId[8:16], shortID[:])

	pubKeyBytes, err := base64.RawURLEncoding.DecodeString(cfg.PublicKeyB64)
	if err != nil {
		return fmt.Errorf("vinereal: decode server public key: %w", err)
	}
	serverPub, err := ecdh.X25519().NewPublicKey(pubKeyBytes)
	if err != nil {
		return fmt.Errorf("vinereal: invalid server public key: %w", err)
	}

	ks := uconn.HandshakeState.State13.KeyShareKeys
	if ks == nil {
		return errors.New("vinereal: no TLS 1.3 key share was generated; fingerprint may not support TLS 1.3")
	}
	ecdhePriv := ks.Ecdhe
	if ecdhePriv == nil {
		ecdhePriv = ks.MlkemEcdhe
	}
	if ecdhePriv == nil {
		return errors.New("vinereal: fingerprint produced no X25519 key share, REALITY handshake cannot proceed")
	}

	shared, err := ecdhePriv.ECDH(serverPub)
	if err != nil {
		return fmt.Errorf("vinereal: ecdh: %w", err)
	}

	authKey, err := hkdf.Key(sha256.New, shared, hello.Random[:20], "REALITY", 32)
	if err != nil {
		return fmt.Errorf("vinereal: hkdf: %w", err)
	}
	a.authKey = authKey

	block, err := aes.NewCipher(authKey)
	if err != nil {
		return fmt.Errorf("vinereal: aes: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return fmt.Errorf("vinereal: gcm: %w", err)
	}

	// In-place seal: dst (hello.SessionId[:0]) and plaintext
	// (hello.SessionId[:16]) share the same backing array, which is the
	// standard "encrypt in place" idiom cipher.AEAD implementations
	// support (dst must equal plaintext[:0] exactly, which it does here).
	aead.Seal(hello.SessionId[:0], hello.Random[20:32], hello.SessionId[:16], hello.Raw)
	copy(hello.Raw[sessionIDOffset:], hello.SessionId)

	return nil
}

// verifyPeerCertificate is installed as utls.Config.VerifyPeerCertificate.
// It runs even with InsecureSkipVerify set, which is what lets us tell
// apart "handshake succeeded against our real REALITY server" from
// "handshake succeeded against the decoy site because our auth token was
// rejected and xtls/reality fell back to splicing the connection through
// to its configured Dest".
func (a *authenticator) verifyPeerCertificate(rawCerts [][]byte, _ [][]*x509.Certificate) error {
	if len(rawCerts) == 0 {
		return errors.New("vinereal: server presented no certificate")
	}
	cert, err := x509.ParseCertificate(rawCerts[0])
	if err != nil {
		return fmt.Errorf("vinereal: parse server certificate: %w", err)
	}
	pub, ok := cert.PublicKey.(ed25519.PublicKey)
	if !ok {
		return errors.New("vinereal: server certificate is not ed25519, not talking to a REALITY server (likely fallback to decoy site)")
	}

	mac := hmac.New(sha512.New, a.authKey)
	mac.Write(pub)
	if !hmac.Equal(mac.Sum(nil), cert.Signature) {
		return errors.New("vinereal: server certificate signature does not match our REALITY auth key (likely fallback to decoy site, or a MITM)")
	}

	a.verified = true
	return nil
}

// decodeShortID hex-decodes a REALITY short ID (0-16 hex chars = 0-8 bytes)
// into a zero-padded [8]byte, matching the server's short-id map key shape.
func decodeShortID(s string) ([8]byte, error) {
	var out [8]byte
	if s == "" {
		return out, nil
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return out, fmt.Errorf("invalid hex: %w", err)
	}
	if len(b) > 8 {
		return out, fmt.Errorf("short id must be at most 8 bytes, got %d", len(b))
	}
	copy(out[:], b)
	return out, nil
}

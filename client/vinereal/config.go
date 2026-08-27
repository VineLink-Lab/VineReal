package vinereal

import "github.com/VineLink-Lab/VineReal/shared/visionframe"

// Config holds everything the client needs to dial a REALITY-fronted
// reverse proxy. In production this is meant to be a compile-time constant
// baked into the app (see `vinereal-server keygen -format go` output), not a
// runtime-editable value the end user ever sees or fills in.
type Config struct {
	// ServerAddr is the host:port of one of the interchangeable reverse-proxy
	// nodes. Multiple nodes can share the same PublicKeyB64/ShortIDHex/
	// ServerName, so operators can swap this value (or ship a small pool of
	// candidates) without touching the rest of the config.
	ServerAddr string

	// ServerName is the decoy site's SNI/hostname, exactly matching one of
	// the server's configured reality.server_names.
	ServerName string

	// PublicKeyB64 is the REALITY server's X25519 public key
	// (base64.RawURLEncoding of the raw 32-byte scalar).
	PublicKeyB64 string

	// ShortIDHex is this client identity's REALITY short ID, hex-encoded
	// (0-16 hex chars, i.e. 0-8 bytes; zero-padded on use). Empty string is
	// the valid "default" short ID.
	ShortIDHex string

	// Fingerprint selects the uTLS ClientHelloID to mimic (e.g.
	// "chrome_auto", "ios_auto", "android_okhttp"). See fingerprint.go.
	Fingerprint string

	// HandshakeTimeoutMS bounds the whole dial+handshake+vision-exchange
	// sequence.
	HandshakeTimeoutMS int64

	// Vision controls this side's post-handshake padding behavior.
	Vision visionframe.Config
}

// DefaultConfig carries sensible non-identity defaults. It deliberately
// leaves ServerAddr/ServerName/PublicKeyB64/ShortIDHex empty — those come
// from a specific deployment's generated config (see `vinereal-server keygen`).
var DefaultConfig = Config{
	Fingerprint:        "chrome_auto",
	HandshakeTimeoutMS: 10_000,
	Vision:             visionframe.DefaultConfig,
}

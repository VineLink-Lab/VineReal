package vinereal

import (
	"fmt"

	utls "github.com/refraction-networking/utls"
)

// fingerprints maps the human-readable names used in Config.Fingerprint to
// uTLS's predefined ClientHelloIDs. Kept as an explicit allowlist (rather
// than exposing utls.ClientHelloID directly) so the SDK's public surface
// stays small and gomobile-friendly (string in, no utls types crossing the
// package boundary).
var fingerprints = map[string]utls.ClientHelloID{
	"chrome_auto":       utls.HelloChrome_Auto,
	"firefox_auto":      utls.HelloFirefox_Auto,
	"safari_auto":       utls.HelloSafari_Auto,
	"ios_auto":          utls.HelloIOS_Auto,
	"edge_auto":         utls.HelloEdge_Auto,
	"android_okhttp":    utls.HelloAndroid_11_OkHttp,
	"randomized":        utls.HelloRandomized,
	"randomized_alpn":   utls.HelloRandomizedALPN,
	"randomized_noalpn": utls.HelloRandomizedNoALPN,
}

func lookupFingerprint(name string) (utls.ClientHelloID, error) {
	if name == "" {
		return fingerprints["chrome_auto"], nil
	}
	id, ok := fingerprints[name]
	if !ok {
		return utls.ClientHelloID{}, fmt.Errorf("vinereal: unknown fingerprint %q", name)
	}
	return id, nil
}

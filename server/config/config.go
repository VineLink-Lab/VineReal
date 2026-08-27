// Package config loads and validates the vinereal-server YAML configuration.
package config

import (
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"os"
	"time"

	"github.com/VineLink-Lab/VineReal/shared/visionframe"
	"gopkg.in/yaml.v3"
)

// Raw mirrors the on-disk YAML shape.
type Raw struct {
	Listen      string     `yaml:"listen"`
	Upstream    string     `yaml:"upstream"`
	UpstreamTLS bool       `yaml:"upstream_tls"`
	Reality     RawReality `yaml:"reality"`
	Vision      *RawVision `yaml:"vision"`
	Debug       bool       `yaml:"debug"`
}

type RawReality struct {
	Dest           string   `yaml:"dest"`
	ServerNames    []string `yaml:"server_names"`
	PrivateKeyB64  string   `yaml:"private_key_b64"`
	ShortIDs       []string `yaml:"short_ids"`
	MinClientVer   string   `yaml:"min_client_ver"`
	MaxClientVer   string   `yaml:"max_client_ver"`
	MaxTimeDiffSec int      `yaml:"max_time_diff_sec"`
}

type RawVision struct {
	MinFrames       int `yaml:"min_frames"`
	MaxFrames       int `yaml:"max_frames"`
	MinPaddingBytes int `yaml:"min_padding_bytes"`
	MaxPaddingBytes int `yaml:"max_padding_bytes"`
	MinDelayMS      int `yaml:"min_delay_ms"`
	MaxDelayMS      int `yaml:"max_delay_ms"`
}

// Config is the parsed, validated, ready-to-use configuration.
type Config struct {
	Listen      string
	Upstream    string
	UpstreamTLS bool

	Dest         string
	ServerNames  map[string]bool
	PrivateKey   []byte // raw 32-byte X25519 scalar
	ShortIDs     map[[8]byte]bool
	MinClientVer []byte
	MaxClientVer []byte
	MaxTimeDiff  time.Duration

	Vision visionframe.Config
	Debug  bool
}

// Load reads, parses and validates the YAML config at path.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	var raw Raw
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return fromRaw(&raw)
}

func fromRaw(raw *Raw) (*Config, error) {
	if raw.Listen == "" {
		return nil, fmt.Errorf("config: listen is required")
	}
	if raw.Upstream == "" {
		return nil, fmt.Errorf("config: upstream is required")
	}
	if raw.Reality.Dest == "" {
		return nil, fmt.Errorf("config: reality.dest is required")
	}
	if len(raw.Reality.ServerNames) == 0 {
		return nil, fmt.Errorf("config: reality.server_names must have at least one entry")
	}
	if raw.Reality.PrivateKeyB64 == "" {
		return nil, fmt.Errorf("config: reality.private_key_b64 is required (see cmd/realitykeygen)")
	}

	privKey, err := base64.RawURLEncoding.DecodeString(raw.Reality.PrivateKeyB64)
	if err != nil {
		return nil, fmt.Errorf("config: reality.private_key_b64: %w", err)
	}
	if len(privKey) != 32 {
		return nil, fmt.Errorf("config: reality.private_key_b64 must decode to 32 bytes, got %d", len(privKey))
	}

	serverNames := make(map[string]bool, len(raw.Reality.ServerNames))
	for _, n := range raw.Reality.ServerNames {
		serverNames[n] = true
	}

	shortIDs := make(map[[8]byte]bool, len(raw.Reality.ShortIDs))
	for _, s := range raw.Reality.ShortIDs {
		id, err := decodeShortID(s)
		if err != nil {
			return nil, fmt.Errorf("config: reality.short_ids %q: %w", s, err)
		}
		shortIDs[id] = true
	}
	if len(shortIDs) == 0 {
		// An explicit empty list is almost certainly a misconfiguration:
		// no client would ever be able to authenticate.
		return nil, fmt.Errorf("config: reality.short_ids must have at least one entry (use \"\" for the all-zero default)")
	}

	cfg := &Config{
		Listen:      raw.Listen,
		Upstream:    raw.Upstream,
		UpstreamTLS: raw.UpstreamTLS,

		Dest:        raw.Reality.Dest,
		ServerNames: serverNames,
		PrivateKey:  privKey,
		ShortIDs:    shortIDs,
		MaxTimeDiff: time.Duration(raw.Reality.MaxTimeDiffSec) * time.Second,

		Vision: visionframe.DefaultConfig,
		Debug:  raw.Debug,
	}

	if raw.Reality.MinClientVer != "" {
		cfg.MinClientVer = []byte(raw.Reality.MinClientVer)
	}
	if raw.Reality.MaxClientVer != "" {
		cfg.MaxClientVer = []byte(raw.Reality.MaxClientVer)
	}

	if raw.Vision != nil {
		cfg.Vision = visionframe.Config{
			MinFrames:       raw.Vision.MinFrames,
			MaxFrames:       raw.Vision.MaxFrames,
			MinPaddingBytes: raw.Vision.MinPaddingBytes,
			MaxPaddingBytes: raw.Vision.MaxPaddingBytes,
			MinDelayMS:      raw.Vision.MinDelayMS,
			MaxDelayMS:      raw.Vision.MaxDelayMS,
		}
	}

	return cfg, nil
}

// decodeShortID hex-decodes a REALITY short ID (0-16 hex chars = 0-8 bytes)
// into a zero-padded [8]byte, matching xtls/reality's own ShortIds map key
// shape.
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

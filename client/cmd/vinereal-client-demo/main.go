// Command vinereal-client-demo is a small test client for a vinereal REALITY
// deployment. It dials the server, performs a single HTTP GET over the
// already-encrypted tunnel, and prints the response.
//
// It is deliberately a test/verification tool: identity material is passed on
// the command line so you can point it at any server. For production, bake the
// values into the SDK instead (see cmd/realitykeygen -format go) and keep this
// binary out of shipped apps.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/VineLink-Lab/VineReal/client/vinereal"
)

// version is stamped at build time via
// -ldflags "-X main.version=$(git describe --tags)" and defaults to "dev" for
// local, untagged builds.
var version = "dev"

func main() {
	var (
		showVersion = flag.Bool("version", false, "print version and exit")
		serverAddr  = flag.String("server", "", "REALITY server host:port")
		serverName  = flag.String("sni", "", "decoy SNI/hostname (must match one of the server's reality.server_names)")
		publicKey   = flag.String("pubkey", "", "REALITY server X25519 public key (base64 raw URL, from realitykeygen)")
		shortID     = flag.String("shortid", "", "REALITY short ID (hex, 0-16 chars; empty = all-zero default)")
		fingerprint = flag.String("fingerprint", "chrome_auto", "uTLS fingerprint name (see client/vinereal/fingerprint.go)")
		timeoutMS   = flag.Int64("timeout", 10000, "dial + handshake + vision-exchange timeout in milliseconds")
		httpHost    = flag.String("host", "", "HTTP Host header to send to the fixed upstream backend")
		httpPath    = flag.String("path", "/", "HTTP request path")
	)
	flag.Parse()

	if *showVersion {
		fmt.Println(version)
		return
	}

	if *serverAddr == "" || *serverName == "" || *publicKey == "" {
		fmt.Fprintln(os.Stderr, "missing required flags: -server, -sni, -pubkey")
		flag.Usage()
		os.Exit(2)
	}

	cfg := vinereal.DefaultConfig
	cfg.ServerAddr = *serverAddr
	cfg.ServerName = *serverName
	cfg.PublicKeyB64 = *publicKey
	cfg.ShortIDHex = *shortID
	cfg.Fingerprint = *fingerprint
	cfg.HandshakeTimeoutMS = *timeoutMS

	client := vinereal.NewClient(cfg)

	// The returned net.Conn is already a REALITY-authenticated TLS tunnel, so
	// we speak plaintext HTTP over it. The addr argument is ignored by design
	// (the destination is fixed by the server operator's upstream config).
	conn, err := client.Dial("tcp", "ignored:0")
	if err != nil {
		log.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
		log.Fatalf("set deadline: %v", err)
	}

	req, err := http.NewRequest(http.MethodGet, "http://vinereal"+*httpPath, nil)
	if err != nil {
		log.Fatalf("build request: %v", err)
	}
	if *httpHost != "" {
		req.Host = *httpHost
	}
	req.Close = true

	if err := req.Write(conn); err != nil {
		log.Fatalf("write request: %v", err)
	}

	resp, err := http.ReadResponse(bufio.NewReader(conn), req)
	if err != nil {
		log.Fatalf("read response: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		log.Fatalf("read body: %v", err)
	}

	fmt.Printf("%s\n", resp.Status)
	fmt.Printf("body (%d bytes):\n%s\n", len(body), body)
}

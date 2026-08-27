package visionframe

import (
	"bytes"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestExchangeThenPassthrough(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	cfg1 := Config{MinFrames: 1, MaxFrames: 3, MinPaddingBytes: 1, MaxPaddingBytes: 32}
	cfg2 := Config{MinFrames: 2, MaxFrames: 5, MinPaddingBytes: 0, MaxPaddingBytes: 16}

	var wg sync.WaitGroup
	wg.Add(2)
	var err1, err2 error
	go func() {
		defer wg.Done()
		err1 = Exchange(c1, cfg1)
	}()
	go func() {
		defer wg.Done()
		err2 = Exchange(c2, cfg2)
	}()

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Exchange did not complete within timeout")
	}

	if err1 != nil {
		t.Fatalf("side 1 Exchange error: %v", err1)
	}
	if err2 != nil {
		t.Fatalf("side 2 Exchange error: %v", err2)
	}

	// After Exchange returns on both ends, the pipe must carry arbitrary
	// follow-up bytes untouched, with no leftover framing.
	want := []byte("hello after vision padding, this is real application data")
	go func() {
		if _, err := c1.Write(want); err != nil {
			t.Errorf("write after exchange: %v", err)
		}
	}()

	got := make([]byte, len(want))
	if _, err := io.ReadFull(c2, got); err != nil {
		t.Fatalf("read after exchange: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("passthrough mismatch: got %q want %q", got, want)
	}
}

func TestRandRange(t *testing.T) {
	for i := 0; i < 1000; i++ {
		v := randRange(5, 5)
		if v != 5 {
			t.Fatalf("degenerate range returned %d, want 5", v)
		}
		v = randRange(10, 3) // hi < lo
		if v != 10 {
			t.Fatalf("inverted range returned %d, want lo=10", v)
		}
		v = randRange(0, 10)
		if v < 0 || v > 10 {
			t.Fatalf("randRange(0,10) out of bounds: %d", v)
		}
	}
}

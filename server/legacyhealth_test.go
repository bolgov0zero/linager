package server

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"testing"
	"time"
)

// The updater of an older server asks the old admin port for a 200 after it starts the new binary; the stub answers it on
// the loopback and goes away with the context.
func TestLegacyHealthAnswersTheOldUpdater(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	_, port, _ := net.SplitHostPort(ln.Addr().String())
	_ = ln.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	// the host of the old key is not the one that is bound: 0.0.0.0 in the file still means the loopback only
	serveLegacyHealth(ctx, "0.0.0.0:"+port, slog.New(slog.NewTextHandler(io.Discard, nil)))

	resp, err := http.Get("http://127.0.0.1:" + port + "/")
	if err != nil {
		t.Fatalf("the old updater would see a dead server: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}

	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 200*time.Millisecond)
		if err != nil {
			return
		}
		_ = c.Close()
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("the stub must stop with the context")
}

func TestLegacyHealthIgnoresAnUnusableAddress(t *testing.T) {
	serveLegacyHealth(context.Background(), "not-an-address", slog.New(slog.NewTextHandler(io.Discard, nil)))
	serveLegacyHealth(context.Background(), "127.0.0.1:0", slog.New(slog.NewTextHandler(io.Discard, nil)))
}

package server

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// legacyHealthWindow is how long after the start the stub below answers.
const legacyHealthWindow = 5 * time.Minute

// serveLegacyHealth answers "200" on the 127.0.0.1 side of the old admin port for a few minutes after the start.
//
// The web panel is gone, but the updater of a server older than 0.10.0-beta.17 is the OLD binary: it starts the new one and
// then asks http://127.0.0.1:<admin port>/ to see that it is healthy, and rolls the update back when nobody answers. So the
// first update to a version without the panel could never succeed. The stub keeps that one question answered; it serves
// nothing else, listens on the loopback only, and stops by itself. It is used only when the old admin.listen key is still in
// server.yaml (an installation made after the removal has none).
func serveLegacyHealth(ctx context.Context, listen string, log *slog.Logger) {
	_, port, err := net.SplitHostPort(listen)
	if err != nil || port == "" || port == "0" {
		return
	}
	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", port))
	if err != nil {
		log.Debug("the stub for the health check of an old updater is not started", "err", err)
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("Linager: the web panel is gone, use the desktop application\n"))
	})
	srv := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		t := time.NewTimer(legacyHealthWindow)
		defer t.Stop()
		select {
		case <-t.C:
		case <-ctx.Done():
		}
		_ = srv.Close()
	}()
	log.Info("answering the health check of an old updater for a few minutes", "addr", ln.Addr().String())
	go func() { _ = srv.Serve(ln) }()
}

// Linager is the desktop application: the interface of a Linager server in native windows. The interface itself
// (frontend/) is shown by the web view of the system; this program keeps the connection to the server (a TLS
// connection pinned to the server certificate), the session and the passwords, and opens the windows.
package main

import (
	"context"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/bolgov0zero/linager/app/internal/local"
	"github.com/bolgov0zero/linager/app/internal/profiles"
)

//go:embed all:frontend/dist
var dist embed.FS

//go:embed build/appicon.png
var appIcon []byte

// version is set at build time (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	headless := flag.Bool("headless", false, "serve the interface on a local address and open no window (development, tests)")
	profilesPath := flag.String("profiles", "", "profiles file (default: <user config dir>/Linager/profiles.json)")
	flag.Parse()

	assets, err := fs.Sub(dist, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}
	path := *profilesPath
	if path == "" {
		if path, err = profiles.DefaultPath(); err != nil {
			log.Fatal(err)
		}
	}
	store, err := profiles.Open(path)
	if err != nil {
		log.Fatalf("profiles: %v", err)
	}
	var secrets profiles.Secrets = profiles.Keychain{}
	if *headless {
		secrets = &profiles.Memory{} // development runs must not touch the keychain of the person
	}

	opts := local.Options{Assets: assets, Profiles: store, Secrets: secrets, Version: version, Debug: os.Getenv("LINAGER_DEBUG") != ""}
	var wins *windows
	if !*headless {
		wins = &windows{open: map[string]*application.WebviewWindow{}}
		opts.OpenWindow = wins.openTerminal
	}
	srv, err := local.New(opts)
	if err != nil {
		log.Fatalf("local server: %v", err)
	}
	go func() {
		if err := srv.Serve(); err != nil {
			log.Fatalf("local server: %v", err)
		}
	}()
	defer srv.Close()

	if *headless {
		fmt.Println(srv.URL())
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		<-ctx.Done()
		return
	}

	if os.Getenv("LINAGER_DEBUG") != "" {
		log.Printf("local server: %s", srv.URL())
	}
	app := application.New(application.Options{
		Name:        "Linager",
		Description: "Linager server management",
		Icon:        appIcon,
		Mac:         application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: true},
		// every window loads the local server; the asset server of the framework has nothing to serve
		Assets: application.AssetOptions{Handler: http.NotFoundHandler()},
	})
	wins.app, wins.base = app, srv.URL()
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "Linager",
		Width:     1320,
		Height:    840,
		MinWidth:  900,
		MinHeight: 560,
		URL:       srv.URL(),
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

// windows keeps the native windows that are open, one per kind and server: asking again raises the window.
type windows struct {
	app  *application.App
	base string
	mu   sync.Mutex
	open map[string]*application.WebviewWindow
}

func (w *windows) openTerminal(kind, id, title string) error {
	key := kind + ":" + id
	w.mu.Lock()
	defer w.mu.Unlock()
	if win, ok := w.open[key]; ok {
		if win.IsMinimised() {
			win.UnMinimise()
		}
		win.Focus()
		return nil
	}
	win := w.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:     "Terminal · " + title,
		Width:     900,
		Height:    560,
		MinWidth:  480,
		MinHeight: 280,
		URL:       w.base + "#/w/" + kind + "/" + id + "?title=" + queryEscape(title),
	})
	w.open[key] = win
	win.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		w.mu.Lock()
		delete(w.open, key)
		w.mu.Unlock()
	})
	return nil
}

func queryEscape(s string) string { return url.QueryEscape(s) }

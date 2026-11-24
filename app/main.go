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
	"path/filepath"
	"sync"
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/bolgov0zero/linager/app/internal/autostart"
	"github.com/bolgov0zero/linager/app/internal/local"
	"github.com/bolgov0zero/linager/app/internal/profiles"
	"github.com/bolgov0zero/linager/app/internal/tray"
	"github.com/bolgov0zero/linager/app/internal/update"
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
	hidden := flag.Bool("hidden", false, "start with the window hidden in the tray (the start at login)")
	after := flag.Int("after", 0, "wait until this process exits before starting (an update starts the new copy this way)")
	flag.Parse()
	if *after > 0 {
		waitExit(*after)
	}

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

	opts := local.Options{Assets: assets, Profiles: store, Prefs: profiles.OpenPrefs(profiles.PrefsPath(path)), Secrets: secrets, VaultMemo: filepath.Join(filepath.Dir(path), "vault-last.json"), Version: version, Debug: os.Getenv("LINAGER_DEBUG") != ""}
	var wins *windows
	var ctl *trayCtl
	prefs := opts.Prefs
	if !*headless {
		update.CleanupOld() // the program that was replaced by the last update could not delete itself
		wins = &windows{open: map[string]*application.WebviewWindow{}}
		cache, cerr := os.UserCacheDir()
		if cerr != nil {
			cache = os.TempDir()
		}
		ctl = newTrayCtl(tray.Open(filepath.Join(filepath.Dir(path), "tray.json")), func() string {
			if prefs.All()["linager.locale"] == "en" {
				return "en"
			}
			return "ru"
		})
		ctl.wins = wins
		opts.Tray, opts.OnTray = ctl.store, ctl.refresh
		if exe, eerr := os.Executable(); eerr == nil {
			opts.AutostartExe = exe
		}
		up, uerr := update.New(version, filepath.Join(cache, "Linager", "update"), ctl.quit)
		if uerr != nil {
			log.Fatalf("update: %v", uerr)
		}
		opts.Update = up
		opts.OpenWindow = wins.show
		opts.PickFile = func(ext string) (string, bool, error) {
			path, err := wins.app.Dialog.OpenFile().AddFilter("Linager keys", "*"+ext).CanChooseFiles(true).CanChooseDirectories(false).PromptForSingleSelection()
			return path, path != "", err
		}
		opts.SaveFile = func(name string) (string, bool, error) {
			path, err := wins.app.Dialog.SaveFile().SetFilename(name).CanCreateDirectories(true).PromptForSingleSelection()
			return path, path != "", err
		}
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
	var app *application.App
	app = application.New(application.Options{
		Name:        "Linager",
		Description: "Linager server management",
		Icon:        appIcon,
		// the program lives in the tray: closing the windows does not end it, "Quit" of the tray and Cmd+Q do
		Mac:     application.MacOptions{ApplicationShouldTerminateAfterLastWindowClosed: false},
		Windows: application.WindowsOptions{DisableQuitOnLastWindowClosed: true},
		Linux:   application.LinuxOptions{DisableQuitOnLastWindowClosed: true},
		ShouldQuit: func() bool {
			ctl.quitting.Store(true)
			return true
		},
		// a second start (a click on the icon, the start at login) shows the window of the first
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID:               autostart.ID,
			OnSecondInstanceLaunch: func(application.SecondInstanceData) { ctl.showMain() },
		},
		// every window loads the local server; the asset server of the framework has nothing to serve
		Assets: application.AssetOptions{Handler: http.NotFoundHandler()},
	})
	wins.app, wins.base = app, srv.URL()
	ctl.app, ctl.srv = app, srv
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Hidden: *hidden,
		Name:   "main",
		Title:  "Linager",
		// the size the person works in is also the smallest: the interface (the top bar, the dashboard, the server page)
		// is laid out for it
		Width:     1340,
		Height:    851,
		MinWidth:  1340,
		MinHeight: 851,
		URL:       srv.URL(),
	})
	ctl.hideOnClose(win)
	ctl.start()
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

// winSpec is the size of a window of a kind and what the person may do with it. Only the terminal, the files, the
// manager and the editor are maximized; the note is a small window.
type winSpec struct {
	title        string
	w, h, mw, mh int
	noMaximise   bool
}

var winSpecs = map[string]winSpec{
	"terminal": {"Terminal", 900, 560, 480, 280, false},
	"files":    {"Files", 900, 560, 560, 320, false},
	"manager":  {"Manager", 820, 560, 520, 320, false},
	"editor":   {"Editor", 820, 560, 480, 280, false},
	"rdp":      {"Remote desktop", 1100, 720, 640, 420, false},
	"asst":     {"Assistant", 1100, 720, 640, 420, false},
	"settings": {"Settings", 1000, 680, 780, 460, false},
	"note":     {"Note", 440, 320, 320, 220, true},
}

func (w *windows) show(kind, id, title, arg string) error {
	sp, ok := winSpecs[kind]
	if !ok {
		return fmt.Errorf("unknown window %q", kind)
	}
	key := kind + ":" + id + ":" + arg
	if kind == "settings" {
		key = "settings" // one window of the settings; a later request only raises it
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if win, ok := w.open[key]; ok {
		if win.IsMinimised() {
			win.UnMinimise()
		}
		win.Focus()
		return nil
	}
	o := application.WebviewWindowOptions{
		Title:     sp.title + " · " + title,
		Width:     sp.w,
		Height:    sp.h,
		MinWidth:  sp.mw,
		MinHeight: sp.mh,
		URL:       w.base + "#/w/" + kind + "/" + url.PathEscape(id) + "?title=" + queryEscape(title) + "&arg=" + queryEscape(arg),
	}
	if sp.noMaximise {
		o.MaximiseButtonState = application.ButtonDisabled
	}
	win := w.app.Window.NewWithOptions(o)
	w.open[key] = win
	win.OnWindowEvent(events.Common.WindowClosing, func(*application.WindowEvent) {
		w.mu.Lock()
		delete(w.open, key)
		w.mu.Unlock()
	})
	return nil
}

func queryEscape(s string) string { return url.QueryEscape(s) }

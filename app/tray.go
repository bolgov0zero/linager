package main

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/bolgov0zero/linager/app/internal/local"
	"github.com/bolgov0zero/linager/app/internal/tray"
)

//go:embed build/tray/tray-template.png
var trayTemplate []byte

//go:embed build/tray/tray-white.png
var trayWhite []byte

//go:embed build/tray/tray-black.png
var trayBlack []byte

//go:embed build/tray/item-terminal.png
var itemTerminal []byte

//go:embed build/tray/item16-terminal.png
var item16Terminal []byte

//go:embed build/tray/item16-rdp.png
var item16RDP []byte

//go:embed build/tray/item16-asst.png
var item16Asst []byte

//go:embed build/tray/item-rdp.png
var itemRDP []byte

//go:embed build/tray/item-asst.png
var itemAsst []byte

// trayPoll is how often the state of the servers in the menu is read.
const trayPoll = 30 * time.Second

// trayCtl is the icon in the tray and its menu: "Open", the state of the servers, the connections chosen in the
// settings of the servers, "Quit". Closing the window of the application hides it here; only "Quit" (or Cmd+Q) ends the
// program.
type trayCtl struct {
	app   *application.App
	wins  *windows
	srv   *local.Server
	store *tray.Store
	lang  func() string // "ru" or "en"
	main  *application.WebviewWindow
	icon  *application.SystemTray

	quitting atomic.Bool
	kick     chan struct{}

	mu   sync.Mutex
	last string // the key of the menu now shown: it is rebuilt only when it changes
}

func newTrayCtl(store *tray.Store, lang func() string) *trayCtl {
	return &trayCtl{store: store, lang: lang, kick: make(chan struct{}, 1)}
}

// refresh asks for the menu to be read again soon (a connection, a server or the chosen entries changed).
func (c *trayCtl) refresh() {
	select {
	case c.kick <- struct{}{}:
	default:
	}
}

// quit ends the program (the window close hook lets the windows go from now on).
func (c *trayCtl) quit() {
	c.quitting.Store(true)
	c.app.Quit()
}

// showMain brings the main window back from the tray.
func (c *trayCtl) showMain() {
	if c.main == nil {
		return
	}
	c.main.Show()
	if c.main.IsMinimised() {
		c.main.UnMinimise()
	}
	c.main.Focus()
}

// hideOnClose makes the close button of the window hide it.
func (c *trayCtl) hideOnClose(win *application.WebviewWindow) {
	c.main = win
	win.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		if c.quitting.Load() {
			return
		}
		e.Cancel()
		win.Hide()
	})
}

// start puts the icon in the tray and starts reading the state.
func (c *trayCtl) start() {
	c.icon = c.app.SystemTray.New()
	if runtime.GOOS == "darwin" {
		c.icon.SetTemplateIcon(trayTemplate)
	} else {
		c.icon.SetIcon(trayBlack)
		c.icon.SetDarkModeIcon(trayWhite)
	}
	c.icon.SetTooltip("Linager")
	// a click of either button opens the menu ("Open" in it brings the window back)
	c.icon.OnClick(c.icon.OpenMenu)
	c.icon.OnRightClick(c.icon.OpenMenu)
	c.render(nil, nil)
	go c.loop()
}

func (c *trayCtl) loop() {
	tk := time.NewTicker(trayPoll)
	defer tk.Stop()
	c.update()
	for {
		select {
		case <-tk.C:
		case <-c.kick:
			time.Sleep(300 * time.Millisecond) // a burst of changes is one read
		}
		if c.quitting.Load() {
			return
		}
		c.update()
	}
}

type dashItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// update reads the dashboard and the waiting agents of the Linager server and draws the menu.
func (c *trayCtl) update() {
	profile := c.srv.Profile()
	if profile == "" {
		c.render(nil, nil)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	body, err := c.srv.Get(ctx, "api/dashboard")
	if err != nil {
		c.render(nil, c.store.List(profile))
		return
	}
	st, err := tray.Summarize(body)
	if err != nil {
		c.render(nil, c.store.List(profile))
		return
	}
	var d struct {
		Items []dashItem `json:"items"`
	}
	if json.Unmarshal(body, &d) == nil {
		for _, it := range d.Items {
			c.store.Rename(profile, it.ID, it.Name)
		}
	}
	// only an administrator may read the list of the agents that wait; for others it is an error and zero
	if raw, err := c.srv.Get(ctx, "api/enrollments"); err == nil {
		var list []json.RawMessage
		if json.Unmarshal(raw, &list) == nil {
			st.Waiting = len(list)
		}
	}
	c.render(&st, c.store.List(profile))
}

// modeIcon is the picture of a connection in the menu: on macOS 32 pixels at 144 dpi (16 points, sharp), elsewhere 16 pixels.
func modeIcon(mode string) []byte {
	big := runtime.GOOS == "darwin"
	pick := func(b, s []byte) []byte {
		if big {
			return b
		}
		return s
	}
	switch mode {
	case "terminal":
		return pick(itemTerminal, item16Terminal)
	case "rdp":
		return pick(itemRDP, item16RDP)
	case "asst":
		return pick(itemAsst, item16Asst)
	}
	return nil
}

// render sets the menu and the mark on the icon. A menu is rebuilt only when what it shows changes, so that it does not
// flicker under the hand of the person.
func (c *trayCtl) render(st *tray.Status, entries []tray.Entry) {
	items := tray.Menu(c.lang(), st, entries)
	attention := 0
	if st != nil {
		attention = st.Attention + st.Waiting
	}
	var key strings.Builder
	key.WriteString(strconv.Itoa(attention))
	for _, it := range items {
		key.WriteString("|" + it.Kind + ":" + it.Label + ":" + it.Mode + ":" + it.ServerID)
	}
	c.mu.Lock()
	if c.last == key.String() {
		c.mu.Unlock()
		return
	}
	c.last = key.String()
	c.mu.Unlock()

	menu := c.app.NewMenu()
	for _, it := range items {
		it := it
		switch it.Kind {
		case "sep":
			menu.AddSeparator()
		case "open":
			menu.Add(it.Label).OnClick(func(*application.Context) { c.showMain() })
		case "quit":
			menu.Add(it.Label).OnClick(func(*application.Context) { c.quit() })
		case "status":
			menu.Add(it.Label).SetEnabled(false)
		case "conn":
			mi := menu.Add(it.Label)
			if b := modeIcon(it.Mode); b != nil {
				mi.SetBitmap(b)
			}
			mi.OnClick(func(*application.Context) { _ = c.wins.show(it.Mode, it.ServerID, it.Name, "") })
		}
	}
	c.icon.SetMenu(menu)
	if runtime.GOOS == "darwin" {
		// a template image takes no colour: the number next to the icon tells that something needs a look
		if attention > 0 {
			c.icon.SetLabel(strconv.Itoa(attention))
		} else {
			c.icon.SetLabel("")
		}
		return
	}
	if attention > 0 {
		c.icon.SetIcon(withDot(trayBlack))
		c.icon.SetDarkModeIcon(withDot(trayWhite))
	} else {
		c.icon.SetIcon(trayBlack)
		c.icon.SetDarkModeIcon(trayWhite)
	}
}

// withDot draws a red dot in the lower right corner of a PNG.
func withDot(src []byte) []byte {
	img, err := png.Decode(bytes.NewReader(src))
	if err != nil {
		return src
	}
	b := img.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, img, b.Min, draw.Src)
	r := b.Dx() / 5
	cx, cy := b.Max.X-r-1, b.Max.Y-r-1
	red := color.RGBA{R: 0xef, G: 0x44, B: 0x44, A: 0xff}
	for y := cy - r; y <= cy+r; y++ {
		for x := cx - r; x <= cx+r; x++ {
			if (x-cx)*(x-cx)+(y-cy)*(y-cy) <= r*r {
				dst.SetRGBA(x, y, red)
			}
		}
	}
	var out bytes.Buffer
	if err := png.Encode(&out, dst); err != nil {
		return src
	}
	return out.Bytes()
}

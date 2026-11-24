// Package autostart starts the application when the person signs in to the computer, hidden in the tray.
package autostart

import (
	"fmt"
	"strings"
)

// the identifier of the application: the name of the LaunchAgent, the value in the registry and the .desktop file
const (
	ID    = "io.github.bolgov0zero.linager"
	Flag  = "-hidden" // the application starts in the tray without opening its window
	title = "Linager"
)

// plist is the LaunchAgent of macOS. A bundle is started through LaunchServices (open), a loose program directly.
func plist(exe string) string {
	args := []string{exe, Flag}
	if i := strings.Index(exe, ".app/Contents/MacOS/"); i > 0 {
		args = []string{"/usr/bin/open", "-g", "-a", exe[:i+4], "--args", Flag}
	}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
  <key>Label</key><string>` + ID + `</string>
  <key>ProgramArguments</key><array>`)
	for _, a := range args {
		b.WriteString("<string>" + xmlEscape(a) + "</string>")
	}
	b.WriteString(`</array>
  <key>RunAtLoad</key><true/>
</dict></plist>
`)
	return b.String()
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
}

// desktopFile is the autostart entry of the Linux desktops (the freedesktop specification).
func desktopFile(exe string) string {
	return fmt.Sprintf("[Desktop Entry]\nType=Application\nName=%s\nExec=%s %s\nX-GNOME-Autostart-enabled=true\nTerminal=false\n", title, quoteExec(exe), Flag)
}

// quoteExec quotes a path for the Exec key (spaces and the characters the specification reserves).
func quoteExec(p string) string {
	if !strings.ContainsAny(p, " \t\"'\\><~|&;$*?#()`") {
		return p
	}
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`, "`", "\\`", "$", `\$`).Replace(p) + `"`
}

// runKey is what is written to the Run key of the registry of Windows.
func runValue(exe string) string { return `"` + exe + `" ` + Flag }

package console

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"

	"github.com/bolgov0zero/linager/installer"
)

// The palette of the installer: adaptive colors read on light and dark terminals.
var (
	cAccent = lipgloss.AdaptiveColor{Light: "#2F5BD8", Dark: "#6E8FF2"}
	cText   = lipgloss.AdaptiveColor{Light: "#1C1C1C", Dark: "#E4E4E4"}
	cGrey   = lipgloss.AdaptiveColor{Light: "#8A8A8A", Dark: "#7C7C7C"}
	cGreen  = lipgloss.AdaptiveColor{Light: "28", Dark: "35"}
	cYellow = lipgloss.AdaptiveColor{Light: "130", Dark: "178"}
	cRed    = lipgloss.AdaptiveColor{Light: "160", Dark: "160"}
)

// logo is drawn with box characters, so it needs no special font.
var logo = []string{
	"╦  ╦╔╗╔╔═╗╔═╗╔═╗╦═╗",
	"║  ║║║║╠═╣║ ╦║╣ ╠╦╝",
	"╩═╝╩╝╚╝╩ ╩╚═╝╚═╝╩╚═",
}

type tone int

const (
	toneNone tone = iota
	toneOK
	toneWarn
	toneBad
)

// kv is one line of a status table.
type kv struct {
	k, v string
	tone tone
}

func style(c lipgloss.TerminalColor) lipgloss.Style { return lipgloss.NewStyle().Foreground(c) }

func dot(t tone) string {
	switch t {
	case toneOK:
		return style(cGreen).Render("●")
	case toneWarn:
		return style(cYellow).Render("●")
	case toneBad:
		return style(cRed).Render("●")
	}
	return " "
}

// table aligns the labels of rows in one column.
func table(rows []kv) string {
	lw := 0
	for _, r := range rows {
		lw = max(lw, lipgloss.Width(r.k))
	}
	var b strings.Builder
	for i, r := range rows {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(style(cGrey).Width(lw).Render(r.k) + "  " + dot(r.tone) + " " + style(cText).Render(r.v))
	}
	return b.String()
}

// header draws the logo with the version, the mode and, when there is one, the available update.
func header(l installer.Lang, version, mode, latest string) string {
	var lg []string
	for _, s := range logo {
		lg = append(lg, style(cAccent).Bold(true).Render(s))
	}
	info := []string{
		style(cText).Bold(true).Render("Linager") + "  " + style(cGrey).Render(tr(l, "ver", version)),
		style(cGrey).Render(tr(l, "mode", mode)),
		"",
	}
	if latest != "" {
		info[2] = style(cYellow).Render(tr(l, "update.new", latest))
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, strings.Join(lg, "\n"), "   ", strings.Join(info, "\n"))
}

// card is a bordered block of the role picker.
func card(title string, rows []kv, w int, selected bool) string {
	bc := cGrey
	if selected {
		bc = cAccent
	}
	body := style(cAccent).Bold(true).Render(title) + "\n\n" + table(rows)
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(bc).Padding(0, 1).Width(w - 2).Render(body)
}

func serviceRow(l installer.Lang, s Service) kv {
	k := tr(l, "s.service")
	switch s.State {
	case "active":
		if !s.Since.IsZero() {
			return kv{k, tr(l, "v.since", span(l, time.Since(s.Since))), toneOK}
		}
		return kv{k, tr(l, "v.running"), toneOK}
	case "activating", "reloading":
		return kv{k, tr(l, "v.starting"), toneWarn}
	case "failed":
		return kv{k, tr(l, "v.failed"), toneBad}
	case "inactive", "deactivating":
		return kv{k, tr(l, "v.stopped"), toneBad}
	case "not-found":
		return kv{k, tr(l, "v.missing"), toneBad}
	}
	return kv{k, tr(l, "v.unknown"), toneWarn}
}

func loadingRows(l installer.Lang, keys ...string) []kv {
	var out []kv
	for _, k := range keys {
		out = append(out, kv{tr(l, k), tr(l, "v.loading"), toneNone})
	}
	return out
}

func backupRow(l installer.Lang, t time.Time, short bool) kv {
	k := tr(l, "s.backup")
	switch {
	case t.IsZero():
		return kv{k, tr(l, "v.nobackup"), toneWarn}
	case short:
		return kv{k, ago(l, time.Since(t)), toneNone}
	}
	return kv{k, join(ago(l, time.Since(t)), t.Format("2006-01-02 15:04")), toneNone}
}

func clientsRow(l installer.Lang, st *ServerStatus, short bool) kv {
	k := tr(l, "s.clients")
	if st.Clients < 0 {
		return kv{k, tr(l, "v.hidden"), toneWarn}
	}
	if st.Pending > 0 {
		key := "v.pending"
		if short {
			key = "v.pending.s"
		}
		return kv{k, join(tr(l, "v.clients", st.Clients), tr(l, key, st.Pending)), toneWarn}
	}
	return kv{k, tr(l, "v.clients", st.Clients), toneNone}
}

// serverRows are the lines of the server table; short keeps only what fits a card.
func serverRows(l installer.Lang, st *ServerStatus, version, latest string, short bool) []kv {
	if st == nil {
		return loadingRows(l, "s.service", "s.clients", "s.backup")
	}
	rows := []kv{serviceRow(l, st.Service)}
	if !short {
		db := kv{tr(l, "s.db"), tr(l, "v.dbok"), toneOK}
		switch {
		case st.NeedRoot:
			db = kv{tr(l, "s.db"), tr(l, "v.hidden"), toneWarn}
		case st.ConfigErr != "":
			db = kv{tr(l, "s.db"), tr(l, "v.dbbad", st.ConfigErr), toneBad}
		case !st.DBOK:
			db = kv{tr(l, "s.db"), tr(l, "v.dbbad", st.DBErr), toneBad}
		}
		rows = append(rows, db)
		if st.AppPort != "" {
			rows = append(rows, kv{tr(l, "s.admin"), st.AppPort, toneNone})
		}
		if st.AgentListen != "" {
			rows = append(rows, kv{tr(l, "s.agents"), st.AgentListen, toneNone})
		}
	}
	rows = append(rows, clientsRow(l, st, short), backupRow(l, st.LastBackup, short))
	if !short {
		if st.Access != "" {
			t := toneNone
			if st.Access != "off" {
				t = toneWarn
			}
			rows = append(rows, kv{tr(l, "s.access"), tr(l, "v.access."+st.Access), t})
		}
		v := kv{tr(l, "s.update"), version, toneNone}
		if latest != "" {
			v = kv{v.k, join(version, tr(l, "update.new", latest)), toneWarn}
		}
		rows = append(rows, v)
	}
	return rows
}

func agentRows(l installer.Lang, st *AgentStatus, version string, short bool) []kv {
	if st == nil {
		return loadingRows(l, "s.service", "s.server", "s.modules")
	}
	srv := kv{tr(l, "s.server"), st.Server, toneOK}
	switch {
	case st.NeedRoot:
		srv = kv{srv.k, tr(l, "v.hidden"), toneWarn}
	case !st.Enrolled:
		srv = kv{srv.k, tr(l, "v.notenrolled"), toneWarn}
	}
	rows := []kv{serviceRow(l, st.Service), srv}
	if st.ModulesOn >= 0 {
		rows = append(rows, kv{tr(l, "s.modules"), tr(l, "v.modules", st.ModulesOn, st.ModulesAll), toneNone})
	}
	if !short {
		rows = append(rows, kv{tr(l, "s.agentver"), version, toneNone})
	}
	return rows
}

// entryLine is one numbered line of a menu.
func entryLine(num, label, tag string, selected bool) string {
	cur := "  "
	if selected {
		cur = style(cAccent).Render("▸ ")
	}
	lbl := style(cText).Render(label)
	if selected {
		lbl = style(cAccent).Bold(true).Render(label)
	}
	line := cur + style(cAccent).Bold(true).Render(num) + "  " + lbl
	if tag != "" {
		line += "  " + style(cGrey).Render(tag)
	}
	return line
}

func hint(s string) string { return style(cGrey).Render(s) }

func fmtBytes(n int64) string { return fmt.Sprintf("%.1f MB", float64(n)/(1<<20)) }

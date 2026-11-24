package server

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/bolgov0zero/linager/internal/protocol"
	"github.com/bolgov0zero/linager/server/store"
)

type periodDef struct {
	d    time.Duration
	tier string
}

// periods maps the UI period to its span and storage tier (see ui-spec).
var periods = map[string]periodDef{
	"1h":   {time.Hour, "raw"},
	"24h":  {24 * time.Hour, "1m"},
	"7d":   {7 * 24 * time.Hour, "1m"},
	"30d":  {30 * 24 * time.Hour, "5m"},
	"90d":  {90 * 24 * time.Hour, "5m"},
	"180d": {180 * 24 * time.Hour, "15m"},
	"360d": {360 * 24 * time.Hour, "1h"},
}

const targetBars = 120

// recentLen is the number of samples behind the overview bars (the UI shows 60 slots).
const recentLen = 60

// binFor picks the bar width: a multiple of the tier step giving at most targetBars bars.
func binFor(span, step time.Duration) time.Duration {
	n := (span/targetBars + step - 1) / step
	if n < 1 {
		n = 1
	}
	return n * step
}

type serverCard struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Address      string           `json:"address"`
	OS           string           `json:"os"`
	AgentVersion string           `json:"agent_version"`
	Online       bool             `json:"online"`
	LastSeenAt   *time.Time       `json:"last_seen_at"`
	Sample       *protocol.Sample `json:"sample"`
	Spark        []float64        `json:"spark"`
	GroupID      *int64           `json:"group_id"`
}

func (a *Admin) card(ctx context.Context, s *store.Server) serverCard {
	v := a.Metrics.Get(ctx, s.ID)
	if v.Spark == nil {
		v.Spark = []float64{}
	}
	return serverCard{ID: s.ID, Name: s.Name, Address: s.Address, OS: s.OS, AgentVersion: s.AgentVersion,
		Online: a.Hub.Online(s.ID), LastSeenAt: s.LastSeenAt, Sample: v.Sample, Spark: v.Spark}
}

func (a *Admin) dashID(r *http.Request, u *store.User) (int64, bool) {
	id, err := a.DB.DashboardID(r.Context(), u.ID)
	return id, err == nil
}

func (a *Admin) dashboard(w http.ResponseWriter, r *http.Request, u *store.User) {
	did, ok := a.dashID(r, u)
	if !ok {
		jsonErr(w, http.StatusInternalServerError, "no dashboard")
		return
	}
	// the dashboard of an operator or an auditor always shows exactly the servers they may see
	if visible, err := a.visibleServers(r, u); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	} else if visible != nil {
		if err := a.DB.DashboardSync(r.Context(), did, visible); err != nil {
			jsonErr(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	list, err := a.DB.DashboardServers(r.Context(), did)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	groups, _ := a.DB.ServerGroups(r.Context())
	items := make([]serverCard, 0, len(list))
	for _, s := range list {
		c := a.card(r.Context(), s)
		if g, ok := groups[s.ID]; ok {
			c.GroupID = &g
		}
		items = append(items, c)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (a *Admin) dashboardAvailable(w http.ResponseWriter, r *http.Request, u *store.User) {
	did, ok := a.dashID(r, u)
	if !ok {
		jsonErr(w, http.StatusInternalServerError, "no dashboard")
		return
	}
	list, err := a.DB.AvailableServers(r.Context(), did)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]map[string]string, 0, len(list))
	for _, s := range list {
		out = append(out, map[string]string{"id": s.ID, "name": s.Name, "address": s.Address, "os": s.OS})
	}
	writeJSON(w, http.StatusOK, out)
}

func (a *Admin) dashboardAdd(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		ServerID string `json:"server_id"`
	}
	if !decode(w, r, &req) {
		return
	}
	did, ok := a.dashID(r, u)
	if !ok {
		jsonErr(w, http.StatusInternalServerError, "no dashboard")
		return
	}
	srv, err := a.DB.ServerByID(r.Context(), req.ServerID)
	if err != nil {
		jsonErr(w, http.StatusNotFound, "server not found")
		return
	}
	if err := a.DB.DashboardAdd(r.Context(), did, srv.ID); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	a.audit(r, u, "dashboard.add", "server", srv.ID, srv.Name, "ok", nil)
	a.Live.Publish("dashboard", map[string]any{})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *Admin) dashboardRemove(w http.ResponseWriter, r *http.Request, u *store.User) {
	did, ok := a.dashID(r, u)
	if !ok {
		jsonErr(w, http.StatusInternalServerError, "no dashboard")
		return
	}
	id := r.PathValue("id")
	name := id
	if srv, err := a.DB.ServerByID(r.Context(), id); err == nil {
		name = srv.Name
	}
	if err := a.DB.DashboardRemove(r.Context(), did, id); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	a.audit(r, u, "dashboard.remove", "server", id, name, "ok", nil)
	a.Live.Publish("dashboard", map[string]any{})
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *Admin) dashboardOrder(w http.ResponseWriter, r *http.Request, u *store.User) {
	var req struct {
		ServerIDs []string `json:"server_ids"`
	}
	if !decode(w, r, &req) {
		return
	}
	did, ok := a.dashID(r, u)
	if !ok {
		jsonErr(w, http.StatusInternalServerError, "no dashboard")
		return
	}
	if err := a.DB.DashboardSetOrder(r.Context(), did, req.ServerIDs); err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	a.audit(r, u, "dashboard.reorder", "dashboard", strconv.FormatInt(did, 10), "", "ok", nil)
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (a *Admin) serverDetail(w http.ResponseWriter, r *http.Request, _ *store.User) {
	s, err := a.DB.ServerByID(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, http.StatusNotFound, "server not found")
		return
	}
	v := a.Metrics.Get(r.Context(), s.ID)
	if v.Spark == nil {
		v.Spark = []float64{}
	}
	mods, _ := a.DB.Modules(r.Context(), s.ID)
	if mods == nil {
		mods = []string{}
	}
	// the overview bars start filled: the last samples come from the stored raw metrics
	recent, err := a.DB.RecentRaw(r.Context(), s.ID, recentLen)
	if err != nil {
		a.Log.Error("recent samples", "err", err)
		recent = []store.RecentPoint{}
	}
	note, _ := a.DB.ServerNote(r.Context(), s.ID)
	tags, _ := a.DB.ServerTags(r.Context(), s.ID)
	writeJSON(w, http.StatusOK, map[string]any{
		"modules": mods, "tags": tags, "recent": recent, "note": noteViewOf(note).Note, "note_updated_at": noteViewOf(note).UpdatedAt, "note_updated_by": note.UpdatedBy,
		"id": s.ID, "name": s.Name, "hostname": s.Hostname, "address": s.Address, "os": s.OS, "agent_version": s.AgentVersion,
		"online": a.Hub.Online(s.ID), "last_seen_at": s.LastSeenAt, "enrolled_at": s.EnrolledAt,
		"system": v.System, "sample": v.Sample, "spark": v.Spark,
	})
}

func (a *Admin) historyParams(w http.ResponseWriter, r *http.Request) (*store.Server, periodDef, string, bool) {
	s, err := a.DB.ServerByID(r.Context(), r.PathValue("id"))
	if err != nil {
		jsonErr(w, http.StatusNotFound, "server not found")
		return nil, periodDef{}, "", false
	}
	name := r.URL.Query().Get("period")
	if name == "" {
		name = "24h"
	}
	p, ok := periods[name]
	if !ok {
		jsonErr(w, http.StatusBadRequest, "unknown period")
		return nil, periodDef{}, "", false
	}
	return s, p, name, true
}

func (a *Admin) history(w http.ResponseWriter, r *http.Request, _ *store.User) {
	s, p, name, ok := a.historyParams(w, r)
	if !ok {
		return
	}
	tier, _ := store.TierByName(p.tier)
	to := time.Now()
	from := to.Add(-p.d)
	bin := binFor(p.d, tier.Step)
	pts, stats, err := a.DB.HistoryQuery(r.Context(), s.ID, tier, from, to, bin)
	if err != nil {
		a.Log.Error("history", "err", err)
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	events, err := a.DB.Events(r.Context(), s.ID, from, to)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	if pts == nil {
		pts = []store.HistoryPoint{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"period": name, "tier": tier.Name, "step_seconds": int64(bin.Seconds()), "from": from.Unix(), "to": to.Unix(),
		"points": pts, "stats": stats, "events": events,
	})
}

func (a *Admin) historyCSV(w http.ResponseWriter, r *http.Request, u *store.User) {
	s, p, name, ok := a.historyParams(w, r)
	if !ok {
		return
	}
	tier, _ := store.TierByName(p.tier)
	to := time.Now()
	pts, _, err := a.DB.HistoryQuery(r.Context(), s.ID, tier, to.Add(-p.d), to, tier.Step)
	if err != nil {
		jsonErr(w, http.StatusInternalServerError, "internal error")
		return
	}
	a.audit(r, u, "metrics.export", "server", s.ID, s.Name, "ok", map[string]any{"period": name, "rows": len(pts)})
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s-%s.csv"`, safeFile(s.Name), name))
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"time_utc", "cpu_avg", "cpu_max", "mem_avg", "mem_max", "disk_avg", "disk_max", "rx_bps_avg", "rx_bps_max", "tx_bps_avg", "tx_bps_max"})
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 2, 64) }
	for _, p := range pts {
		_ = cw.Write([]string{time.Unix(p.TS, 0).UTC().Format(time.RFC3339), f(p.CPUAvg), f(p.CPUMax), f(p.MemAvg), f(p.MemMax),
			f(p.DiskAvg), f(p.DiskMax), f(p.RxAvg), f(p.RxMax), f(p.TxAvg), f(p.TxMax)})
	}
	cw.Flush()
}

func safeFile(s string) string {
	out := []rune{}
	for _, c := range s {
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c == '.' {
			out = append(out, c)
		} else {
			out = append(out, '_')
		}
	}
	return string(out)
}

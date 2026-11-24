package server

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Host names in the lists of the access restriction (for a home connection with a dynamic address and a DDNS
// name). A name is resolved in the background once a minute; the addresses it points to are judged like listed
// addresses. The last good answer is kept (and stored), so a DNS outage does not lock the admin out, but only for
// hostStaleAfter: a name that has been gone for longer must not keep an address open for ever.

const (
	hostRefreshEvery  = time.Minute
	hostStaleAfter    = 24 * time.Hour
	hostLookupTimeout = 5 * time.Second
	hostCheckMaxAge   = 30 * time.Second // a draft check does not ask DNS again for a name looked up this recently
	hostPruneAfter    = 10 * time.Minute // a looked-up name that is in no list is forgotten after this time
	hostPersistEvery  = 30 * time.Minute // the time of the last good answer is written to the database at most this often
	hostLookupWorkers = 8
	maxHostAddrs      = 16
	maxAccessHosts    = 20
	hostsSetting      = "access_hosts"
)

// HostResolver resolves a host name to addresses (net.DefaultResolver does).
type HostResolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

type resolverBox struct{ r HostResolver }

var resolverPtr atomic.Pointer[resolverBox]

func currentResolver() HostResolver {
	if b := resolverPtr.Load(); b != nil {
		return b.r
	}
	return net.DefaultResolver
}

// SetHostResolver replaces the resolver of the access restriction (for tests); nil brings the system one back.
func SetHostResolver(r HostResolver) {
	if r == nil {
		resolverPtr.Store(nil)
		return
	}
	resolverPtr.Store(&resolverBox{r})
}

// hostAddrs maps a host name to the addresses that are in force for it now.
type hostAddrs map[string][]netip.Addr

// normalizeHost returns the lower-case form of a host name ("Home.Example.org." is "home.example.org"). Only ASCII
// names with at least one dot are accepted (an IDN is written in its xn-- form); a name whose last label starts with
// a digit is refused, because it is an address, not a name.
func normalizeHost(s string) (string, bool) {
	s = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(s), "."))
	if s == "" || len(s) > 253 || !strings.Contains(s, ".") {
		return "", false
	}
	labels := strings.Split(s, ".")
	for _, l := range labels {
		if l == "" || len(l) > 63 || l[0] == '-' || l[len(l)-1] == '-' {
			return "", false
		}
		for i := 0; i < len(l); i++ {
			c := l[i]
			if (c < 'a' || c > 'z') && (c < '0' || c > '9') && c != '-' {
				return "", false
			}
		}
	}
	if last := labels[len(labels)-1]; last[0] >= '0' && last[0] <= '9' {
		return "", false
	}
	return s, true
}

// resolveHost looks a name up and returns its usable addresses (sorted, at most maxHostAddrs).
func resolveHost(ctx context.Context, host string) ([]netip.Addr, error) {
	ctx, cancel := context.WithTimeout(ctx, hostLookupTimeout)
	defer cancel()
	found, err := currentResolver().LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, err
	}
	seen := map[netip.Addr]bool{}
	out := make([]netip.Addr, 0, len(found))
	for _, a := range found {
		a = a.Unmap().WithZone("")
		if !a.IsValid() || a.IsUnspecified() || a.IsMulticast() || seen[a] {
			continue
		}
		seen[a] = true
		out = append(out, a)
	}
	if len(out) == 0 {
		return nil, errors.New("no usable addresses")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Less(out[j]) })
	if len(out) > maxHostAddrs {
		out = out[:maxHostAddrs]
	}
	return out, nil
}

// hostEntry is what is known about one name.
type hostEntry struct {
	addrs []netip.Addr
	at    time.Time // the last good answer
	tried time.Time // the last attempt
	fail  bool      // the last attempt failed
}

func (e *hostEntry) usable(now time.Time) []netip.Addr {
	if e == nil || e.at.IsZero() || now.Sub(e.at) >= hostStaleAfter {
		return nil
	}
	return e.addrs
}

// HostStatus is what the interface shows beside a name.
type HostStatus struct {
	IPs       []string `json:"ips"`
	CheckedAt int64    `json:"checked_at"` // unix time of the last good answer, 0 when there was none
	Failing   bool     `json:"failing"`    // the last attempt failed (the last known addresses stay in force)
	Expired   bool     `json:"expired"`    // the last known addresses are too old and no longer in force
}

// addrs returns the addresses in force for every known name.
func (g *AccessGuard) addrs() hostAddrs {
	now := time.Now()
	g.hmu.Lock()
	defer g.hmu.Unlock()
	out := make(hostAddrs, len(g.hosts))
	for h, e := range g.hosts {
		if a := e.usable(now); len(a) > 0 {
			out[h] = append([]netip.Addr(nil), a...)
		}
	}
	return out
}

// hostStatuses describes the given names.
func (g *AccessGuard) hostStatuses(names []string) map[string]HostStatus {
	now := time.Now()
	g.hmu.Lock()
	defer g.hmu.Unlock()
	out := make(map[string]HostStatus, len(names))
	for _, h := range names {
		st := HostStatus{IPs: []string{}}
		if e := g.hosts[h]; e != nil {
			for _, a := range e.usable(now) {
				st.IPs = append(st.IPs, a.String())
			}
			if !e.at.IsZero() {
				st.CheckedAt = e.at.Unix()
			}
			st.Failing = e.fail
			st.Expired = !e.at.IsZero() && len(st.IPs) == 0
		}
		out[h] = st
	}
	return out
}

// lookupHosts resolves the names whose last attempt is older than maxAge (0: all of them). A failed attempt keeps the
// last good answer.
func (g *AccessGuard) lookupHosts(ctx context.Context, names []string, maxAge time.Duration) {
	now := time.Now()
	var todo []string
	g.hmu.Lock()
	for _, h := range names {
		if e := g.hosts[h]; e != nil && maxAge > 0 && now.Sub(e.tried) < maxAge {
			continue
		}
		todo = append(todo, h)
	}
	g.hmu.Unlock()
	if len(todo) == 0 {
		return
	}
	type result struct {
		addrs []netip.Addr
		err   error
	}
	res := make([]result, len(todo))
	sem := make(chan struct{}, hostLookupWorkers)
	var wg sync.WaitGroup
	for i, h := range todo {
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			res[i].addrs, res[i].err = resolveHost(ctx, h)
		}()
	}
	wg.Wait()
	done := time.Now()
	g.hmu.Lock()
	defer g.hmu.Unlock()
	for i, h := range todo {
		e := g.hosts[h]
		if e == nil {
			e = &hostEntry{}
			g.hosts[h] = e
		}
		e.tried = done
		if res[i].err != nil {
			e.fail = true
			continue
		}
		e.addrs, e.at, e.fail = res[i].addrs, done, false
	}
}

// pruneHosts forgets the names that are in no list (they were only looked up by a draft check).
func (g *AccessGuard) pruneHosts(keep []string) {
	want := make(map[string]bool, len(keep))
	for _, h := range keep {
		want[h] = true
	}
	now := time.Now()
	g.hmu.Lock()
	defer g.hmu.Unlock()
	for h, e := range g.hosts {
		if !want[h] && now.Sub(e.tried) >= hostPruneAfter {
			delete(g.hosts, h)
		}
	}
}

// refreshHosts is the periodic job: ask DNS about the names of the lists and put the new addresses in force.
func (g *AccessGuard) refreshHosts(ctx context.Context) {
	names := g.trackedHosts() // the lists of the admin panel and of the application port
	g.pruneHosts(names)
	if len(names) == 0 {
		return
	}
	g.lookupHosts(ctx, names, 0)
	g.reapply()
	g.reapplyApp()
	g.persistHosts(ctx, names)
}

func (g *AccessGuard) hostLoop(ctx context.Context) {
	t := time.NewTicker(hostRefreshEvery)
	defer t.Stop()
	g.refreshHosts(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			g.refreshHosts(ctx)
		}
	}
}

// ---- the last good answers survive a restart ----

type storedHost struct {
	IPs []string `json:"ips"`
	At  int64    `json:"at"`
}

func (g *AccessGuard) loadHosts(ctx context.Context) {
	raw := g.db.SettingRaw(ctx, hostsSetting)
	if len(raw) == 0 {
		return
	}
	var in map[string]storedHost
	if err := json.Unmarshal(raw, &in); err != nil {
		g.log.Warn("access restriction: stored host addresses are unreadable, they are looked up again", "err", err)
		return
	}
	g.hmu.Lock()
	defer g.hmu.Unlock()
	for h, s := range in {
		if _, ok := normalizeHost(h); !ok || s.At <= 0 {
			continue
		}
		e := &hostEntry{at: time.Unix(s.At, 0)}
		for _, ip := range s.IPs {
			if a, err := netip.ParseAddr(ip); err == nil {
				e.addrs = append(e.addrs, a.Unmap())
			}
		}
		if len(e.addrs) > 0 {
			g.hosts[h] = e
		}
	}
}

// persistHosts writes the answers of the names in the lists when they changed, and now and then to keep the time of
// the last good answer fresh.
func (g *AccessGuard) persistHosts(ctx context.Context, names []string) {
	out := make(map[string]storedHost, len(names))
	g.hmu.Lock()
	for _, h := range names {
		e := g.hosts[h]
		if e == nil || e.at.IsZero() || len(e.addrs) == 0 {
			continue
		}
		s := storedHost{At: e.at.Unix()}
		for _, a := range e.addrs {
			s.IPs = append(s.IPs, a.String())
		}
		out[h] = s
	}
	g.hmu.Unlock()
	// the comparison ignores the time, so that a minute-by-minute refresh does not write every minute
	same := make(map[string]storedHost, len(out))
	for h, s := range out {
		same[h] = storedHost{IPs: s.IPs}
	}
	key, _ := json.Marshal(same)
	g.hmu.Lock()
	unchanged := string(key) == g.persistedKey && time.Since(g.persistedAt) < hostPersistEvery
	g.hmu.Unlock()
	if unchanged {
		return
	}
	raw, err := json.Marshal(out)
	if err != nil {
		return
	}
	if err := g.db.SetSettingRaw(ctx, hostsSetting, raw); err != nil {
		if ctx.Err() == nil {
			g.log.Warn("access restriction: storing host addresses failed", "err", err)
		}
		return
	}
	g.hmu.Lock()
	g.persistedKey, g.persistedAt = string(key), time.Now()
	g.hmu.Unlock()
}

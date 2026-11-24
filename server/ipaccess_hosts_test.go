package server

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeDNS answers from a table; a name that is not in it does not exist, and fail makes every lookup fail.
type fakeDNS struct {
	mu   sync.Mutex
	m    map[string][]string
	fail bool
	n    int
}

func (f *fakeDNS) set(host string, ips ...string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.m == nil {
		f.m = map[string][]string{}
	}
	f.m[host] = ips
}

func (f *fakeDNS) setFail(v bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fail = v
}

func (f *fakeDNS) lookups() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.n
}

func (f *fakeDNS) LookupNetIP(_ context.Context, _, host string) ([]netip.Addr, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.n++
	if f.fail {
		return nil, errors.New("dns down")
	}
	ips, ok := f.m[host]
	if !ok {
		return nil, errors.New("no such host")
	}
	var out []netip.Addr
	for _, s := range ips {
		out = append(out, netip.MustParseAddr(s))
	}
	return out, nil
}

func testGuard() *AccessGuard {
	return &AccessGuard{log: slog.New(slog.NewTextHandler(io.Discard, nil)), hosts: map[string]*hostEntry{}}
}

func TestNormalizeHost(t *testing.T) {
	ok := map[string]string{
		"home.example.duckdns.org":  "home.example.duckdns.org",
		" Home.Example.ORG. ":       "home.example.org",
		"nas.lan":                   "nas.lan",
		"xn--80aswg.xn--p1ai":       "xn--80aswg.xn--p1ai",
		"a-b.c-d.example.net":       "a-b.c-d.example.net",
		"my-server.dyn.example.com": "my-server.dyn.example.com",
	}
	for in, want := range ok {
		if got, good := normalizeHost(in); !good || got != want {
			t.Errorf("%q: got %q %v, want %q", in, got, good, want)
		}
	}
	long := strings.Repeat("a", 64) + ".example.org"
	for _, in := range []string{"", "nas", "localhost", "1.2.3.4", "203.0.113.0/24", "2001:db8::1", "-a.example.org", "a-.example.org", "a..example.org", "a b.example.org", "пример.рф", "host.example.org:80", "host.example.org/x", "a.b.4", long, strings.Repeat("a.", 130) + "org"} {
		if got, good := normalizeHost(in); good {
			t.Errorf("%q must be refused, got %q", in, got)
		}
	}
}

func TestHostRulesInLists(t *testing.T) {
	hosts := hostAddrs{"home.example.org": {netip.MustParseAddr("203.0.113.7"), netip.MustParseAddr("2001:db8::7")}}
	cfg := AccessConfig{Mode: AccessWhite, White: rules("192.168.1.0/24", "Home.Example.org", "home.example.org.")}
	norm, ar, err := compileAccess(cfg, hosts)
	if err != nil {
		t.Fatal(err)
	}
	if len(norm.White) != 2 || norm.White[1].CIDR != "home.example.org" {
		t.Fatalf("names are lower-cased and deduplicated: %+v", norm.White)
	}
	if len(ar.hosts) != 1 || len(ar.unresolved) != 0 {
		t.Fatalf("hosts %v unresolved %v", ar.hosts, ar.unresolved)
	}
	lan := netip.MustParseAddr("192.168.1.40")
	if v := ar.decide(netip.MustParseAddr("203.0.113.7"), ""); !v.Allowed || v.Reason != reasonInWhite {
		t.Errorf("the address of the name must pass: %+v", v)
	}
	if v := ar.decide(netip.MustParseAddr("2001:db8::7"), ""); !v.Allowed {
		t.Errorf("the IPv6 address of the name must pass: %+v", v)
	}
	if v := ar.decide(netip.MustParseAddr("203.0.113.8"), ""); v.Allowed {
		t.Errorf("a neighbour of the address must not pass: %+v", v)
	}
	if got := ar.problems(lan, ""); len(got) != 0 {
		t.Errorf("unexpected problems %+v", got)
	}

	// a name without an address is a problem, and in a black list it bans nobody
	_, ar, err = compileAccess(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(ar.unresolved) != 1 || ar.unresolved[0] != "home.example.org" {
		t.Fatalf("unresolved %v", ar.unresolved)
	}
	found := false
	for _, p := range ar.problems(lan, "") {
		if p.Code == problemHostNoAddr && p.Host == "home.example.org" {
			found = true
		}
	}
	if !found {
		t.Errorf("an unresolved name must be reported: %+v", ar.problems(lan, ""))
	}
	black := mustRulesHosts(t, AccessConfig{Mode: AccessBlack, Black: rules("bad.example.org")}, hostAddrs{"bad.example.org": {netip.MustParseAddr("198.51.100.9")}})
	if v := black.decide(netip.MustParseAddr("198.51.100.9"), ""); v.Allowed || v.Reason != reasonInBlack {
		t.Errorf("the address of a black-listed name must be refused: %+v", v)
	}
	if v := black.decide(netip.MustParseAddr("198.51.100.10"), ""); !v.Allowed {
		t.Errorf("other addresses pass: %+v", v)
	}

	// names are for the two lists only
	if _, _, err := compileAccess(AccessConfig{Mode: AccessOff, Proxies: rules("proxy.example.org")}, nil); err == nil {
		t.Error("a host name among the trusted proxies must be refused")
	}
	if _, _, err := compileAccess(AccessConfig{Mode: AccessOff, White: rules("1.2.3.4/33")}, nil); err == nil {
		t.Error("a broken address must still be refused")
	}
	var many []string
	for i := 0; i <= maxAccessHosts; i++ {
		many = append(many, "h"+strings.Repeat("x", i)+".example.org")
	}
	if _, _, err := compileAccess(AccessConfig{Mode: AccessWhite, White: rules(many...)}, nil); err == nil {
		t.Error("too many names must be refused")
	}
}

func mustRulesHosts(t *testing.T, cfg AccessConfig, h hostAddrs) *accessRules {
	t.Helper()
	_, ar, err := compileAccess(cfg, h)
	if err != nil {
		t.Fatal(err)
	}
	return ar
}

func TestResolveHostFilters(t *testing.T) {
	dns := &fakeDNS{}
	dns.set("h.example.org", "::ffff:203.0.113.7", "203.0.113.7", "0.0.0.0", "224.0.0.1", "fe80::1%eth0", "198.51.100.1")
	SetHostResolver(dns)
	defer SetHostResolver(nil)
	got, err := resolveHost(context.Background(), "h.example.org")
	if err != nil {
		t.Fatal(err)
	}
	var s []string
	for _, a := range got {
		s = append(s, a.String())
	}
	if strings.Join(s, " ") != "198.51.100.1 203.0.113.7 fe80::1" {
		t.Errorf("mapped form unmapped and deduplicated, unspecified and multicast dropped, zone stripped: %v", s)
	}
	dns.set("empty.example.org", "0.0.0.0")
	if _, err := resolveHost(context.Background(), "empty.example.org"); err == nil {
		t.Error("an answer without a usable address is a failure")
	}
}

func TestLookupHostsKeepsLastGoodAnswer(t *testing.T) {
	dns := &fakeDNS{}
	dns.set("home.example.org", "203.0.113.7")
	SetHostResolver(dns)
	defer SetHostResolver(nil)
	g := testGuard()
	ctx := context.Background()
	names := []string{"home.example.org", "gone.example.org"}

	g.lookupHosts(ctx, names, 0)
	st := g.hostStatuses(names)
	if got := st["home.example.org"]; len(got.IPs) != 1 || got.IPs[0] != "203.0.113.7" || got.Failing || got.CheckedAt == 0 {
		t.Fatalf("good answer: %+v", got)
	}
	if got := st["gone.example.org"]; len(got.IPs) != 0 || !got.Failing || got.CheckedAt != 0 {
		t.Fatalf("a name that does not exist: %+v", got)
	}

	// the provider gave the host a new address
	dns.set("home.example.org", "203.0.113.99")
	g.lookupHosts(ctx, names, 0)
	if a := g.addrs()["home.example.org"]; len(a) != 1 || a[0].String() != "203.0.113.99" {
		t.Fatalf("the new address must replace the old one: %v", a)
	}

	// a recent lookup is not repeated for a draft check, a forced one is
	n := dns.lookups()
	g.lookupHosts(ctx, names, time.Hour)
	if dns.lookups() != n {
		t.Error("a name looked up a moment ago must not be asked again")
	}
	g.lookupHosts(ctx, names, 0)
	if dns.lookups() == n {
		t.Error("maxAge 0 must ask again")
	}

	// DNS goes down: the last good answer stays in force
	dns.setFail(true)
	g.lookupHosts(ctx, names, 0)
	got := g.hostStatuses(names)["home.example.org"]
	if len(got.IPs) != 1 || got.IPs[0] != "203.0.113.99" || !got.Failing || got.Expired {
		t.Fatalf("DNS outage: %+v", got)
	}

	// ... but not for ever
	g.hmu.Lock()
	g.hosts["home.example.org"].at = time.Now().Add(-hostStaleAfter - time.Minute)
	g.hmu.Unlock()
	got = g.hostStatuses(names)["home.example.org"]
	if len(got.IPs) != 0 || !got.Expired {
		t.Fatalf("an answer older than a day must stop counting: %+v", got)
	}
	if len(g.addrs()) != 0 {
		t.Errorf("an expired answer must not reach the rules: %v", g.addrs())
	}

	// a name that is in no list is forgotten after a while
	g.hmu.Lock()
	g.hosts["gone.example.org"].tried = time.Now().Add(-hostPruneAfter - time.Minute)
	g.hmu.Unlock()
	g.pruneHosts([]string{"home.example.org"})
	g.hmu.Lock()
	_, kept := g.hosts["gone.example.org"]
	g.hmu.Unlock()
	if kept {
		t.Error("an unused name must be pruned")
	}
}

// pairDNS fails the common lookup (the pair of A and AAAA questions, the way a poor DNS proxy does) and answers a single family.
type pairDNS struct {
	mu    sync.Mutex
	asked []string
}

func (p *pairDNS) LookupNetIP(_ context.Context, network, host string) ([]netip.Addr, error) {
	p.mu.Lock()
	p.asked = append(p.asked, network)
	p.mu.Unlock()
	switch network {
	case "ip4":
		return []netip.Addr{netip.MustParseAddr("77.88.55.242")}, nil
	case "ip6":
		return nil, errors.New("lookup " + host + ": i/o timeout")
	}
	return nil, errors.New("lookup " + host + ": i/o timeout")
}

func TestResolveHostFallsBackToOneFamilyAtATime(t *testing.T) {
	dns := &pairDNS{}
	SetHostResolver(dns)
	defer SetHostResolver(nil)
	got, err := resolveHost(context.Background(), "ya.ru")
	if err != nil || len(got) != 1 || got[0].String() != "77.88.55.242" {
		t.Fatalf("one family is enough: %v %v", got, err)
	}
	if strings.Join(dns.asked, ",") != "ip,ip4,ip6" {
		t.Fatalf("the common lookup first, then each family: %v", dns.asked)
	}
	// every question fails: the error of DNS comes back
	SetHostResolver(&fakeDNS{fail: true})
	if _, err := resolveHost(context.Background(), "ya.ru"); err == nil || !strings.Contains(err.Error(), "dns down") {
		t.Fatalf("the error of DNS: %v", err)
	}
}

// Command linager-sim is a development tool: it enrolls fake agents against a local server,
// feeds them synthetic metrics and optionally backfills long history directly into the database.
// It is not part of the released binary.
package main

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/http/cookiejar"
	"os"
	"os/signal"
	"time"

	"github.com/bolgov0zero/linager/agent"
	"github.com/bolgov0zero/linager/internal/pki"
	"github.com/bolgov0zero/linager/internal/protocol"
	"github.com/bolgov0zero/linager/server/store"
)

type persona struct {
	name, os              string
	cpu, mem, disk        float64
	updates, failed       int
	reboot, offline, wavy bool
}

var personas = []persona{
	{"web-01", "", 36, 52, 41, 3, 0, false, false, true},
	{"web-02", "", 27, 49, 44, 12, 0, true, false, true},
	{"db-primary", "", 46, 78, 63, 0, 0, false, false, true},
	{"db-replica", "", 21, 71, 63, 5, 1, false, false, true},
	{"cache-01", "", 11, 65, 18, 2, 0, false, false, true},
	{"mail-01", "", 0, 0, 0, 7, 0, false, true, false},
	{"build-runner", "", 93, 86, 72, 8, 0, false, false, false},
	{"backup-nas", "", 4, 31, 91, 1, 0, false, false, false},
	{"proxy-01", "", 9, 24, 29, 0, 0, false, false, true},
	{"monitoring", "", 22, 58, 47, 1, 0, false, false, true},
}

// winPersonas are what -windows enrolls instead: the simulator answers like a Windows host (agent.SetDevPlatform).
var winPersonas = []persona{
	{"office-pc", "Windows 11 Pro 24H2", 18, 61, 55, 4, 0, true, false, true},
	{"dc-01", "Windows Server 2022 Standard", 14, 47, 38, 2, 0, false, false, true},
}

type fake struct {
	p    persona
	boot int64
}

func (f *fake) Collect(now time.Time) protocol.Sample {
	w := 0.0
	if f.p.wavy {
		w = 8 * math.Sin(float64(now.Unix())/20)
	}
	cl := func(v float64) float64 { return math.Max(0, math.Min(100, v)) }
	root, second, iface, load := "/", "/data", "ens18", [3]float64{f.p.cpu / 25, f.p.cpu / 30, f.p.cpu / 35}
	if f.p.os != "" {
		root, second, iface, load = "C:", "D:", "Ethernet", [3]float64{} // Windows has no load average
	}
	return protocol.Sample{TS: now.Unix(), CPU: cl(f.p.cpu + w), MemPct: cl(f.p.mem + w/4), MemTotal: 8 << 30, MemUsed: uint64(cl(f.p.mem) / 100 * (8 << 30)),
		DiskPct: f.p.disk, BootTime: f.boot, Uptime: now.Unix() - f.boot, Load: load,
		RxBps: 3e5 + 1e6*(1+math.Sin(float64(now.Unix())/9)), TxBps: 1e5 + 4e5*(1+math.Cos(float64(now.Unix())/7)),
		Updates: f.p.updates, FailedServices: f.p.failed, RebootRequired: f.p.reboot, Procs: 120, DesktopOff: f.p.os != "" && agent.DevDesktopOff(), Gui: f.p.os == "" && f.p.name == "monitoring",
		Disks: []protocol.Disk{{Mount: root, Device: "/dev/vda1", FS: "ext4", Total: 100 << 30, Used: uint64(f.p.disk) << 30, Pct: f.p.disk},
			{Mount: second, Device: "/dev/vdb1", FS: "xfs", Total: 500 << 30, Used: 120 << 30, Pct: 24}},
		Net: []protocol.NetIface{{Iface: iface, Up: true, IP: "10.0.1.10", MAC: "52:54:00:aa:bb:cc", SpeedMbps: 1000, RxBps: 1e6, TxBps: 4e5, RxTotal: 9 << 30, TxTotal: 3 << 30}}}
}

func (f *fake) osName() string {
	if f.p.os != "" {
		return f.p.os
	}
	return "Debian GNU/Linux 12 (bookworm)"
}

func (f *fake) cpuModel() string {
	if f.p.os != "" {
		return "Intel(R) Core(TM) i7-12700 CPU @ 2.10GHz"
	}
	return "Intel(R) Xeon(R) CPU E5-2680 v4 @ 2.40GHz"
}

func (f *fake) kernel() string {
	if f.p.os != "" {
		return "10.0.26100"
	}
	return "6.1.0-18-amd64"
}

func (f *fake) System(v string) protocol.SystemInfo {
	return protocol.SystemInfo{Hostname: f.p.name, OS: f.osName(), Kernel: f.kernel(), Arch: "amd64", CPUModel: f.cpuModel(), CPUCores: 8, MemTotal: 8 << 30, AgentVersion: v}
}

func main() {
	serverAddr := flag.String("server", "127.0.0.1:9443", "agent port")
	adminURL := flag.String("admin", "https://127.0.0.1:9444", "address of the application port (the API of the server)")
	login := flag.String("login", "admin", "admin login")
	password := flag.String("password", "", "admin password")
	dsn := flag.String("dsn", "", "database DSN for history backfill (optional)")
	windows := flag.Bool("windows", false, "enroll a simulated Windows host instead of the Linux ones")
	flag.Parse()
	if *windows {
		agent.SetDevPlatform("windows")
		personas = winPersonas
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	jar, _ := cookiejar.New(nil)
	// a development tool: the application port presents the certificate of the server itself, which is not checked here
	hc := &http.Client{Jar: jar, Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13}}} //nolint:gosec // development simulator
	var csrf string
	call := func(method, path string, body any, out any) error {
		var rd io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			rd = bytes.NewReader(b)
		}
		req, _ := http.NewRequestWithContext(ctx, method, *adminURL+path, rd)
		req.Header.Set("X-CSRF-Token", csrf)
		resp, err := hc.Do(req)
		if err != nil {
			return err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= 300 {
			b, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("%s %s: %s %s", method, path, resp.Status, b)
		}
		if out != nil {
			return json.NewDecoder(resp.Body).Decode(out)
		}
		return nil
	}
	var lr struct{ CSRF string }
	if err := call("POST", "/api/login", map[string]string{"login": *login, "password": *password}, &lr); err != nil {
		fmt.Fprintln(os.Stderr, "login:", err)
		os.Exit(1)
	}
	csrf = lr.CSRF

	for _, p := range personas {
		dir, _ := os.MkdirTemp("", "sim-"+p.name)
		keyCh := make(chan string, 1)
		done := make(chan error, 1)
		go func() {
			done <- agent.Enroll(ctx, agent.EnrollOptions{Server: *serverAddr, DataDir: dir, AgentVersion: "0.3.2", PollEvery: 300 * time.Millisecond,
				Confirm: func(string) bool { return true },
				Show:    func(k, _, _ string) { keyCh <- k }})
		}()
		key := <-keyCh
		_ = pki.EnrollmentKey // key format is produced by the agent
		var pend []struct{ ID, Hostname string }
		for i := 0; i < 50 && len(pend) == 0; i++ {
			_ = call("GET", "/api/enrollments", nil, &pend)
			time.Sleep(100 * time.Millisecond)
		}
		// pick the request that belongs to this agent (the newest pending one)
		if err := call("POST", "/api/enrollments/"+pend[len(pend)-1].ID+"/approve", map[string]string{"key": key}, nil); err != nil {
			fmt.Fprintln(os.Stderr, "approve:", err)
			os.Exit(1)
		}
		if err := <-done; err != nil {
			fmt.Fprintln(os.Stderr, "enroll:", err)
			os.Exit(1)
		}
		fmt.Println("enrolled", p.name, "->", dir)
		if p.offline {
			continue
		}
		f := &fake{p: p, boot: time.Now().Add(-40 * 24 * time.Hour).Unix()}
		go func() {
			_ = agent.Run(ctx, agent.RunOptions{DataDir: dir, AgentVersion: "0.3.2", Log: log, Collector: f, SampleEvery: 2 * time.Second})
		}()
	}

	if *dsn != "" {
		db, err := store.Open(ctx, *dsn)
		if err != nil {
			fmt.Fprintln(os.Stderr, "db:", err)
			os.Exit(1)
		}
		if err := backfill(ctx, db); err != nil {
			fmt.Fprintln(os.Stderr, "backfill:", err)
			os.Exit(1)
		}
		fmt.Println("history backfilled")
	}
	fmt.Println("simulator running; Ctrl+C to stop")
	<-ctx.Done()
}

// backfill writes synthetic aggregates with a daily rhythm for every tier except raw.
func backfill(ctx context.Context, db *store.Store) error {
	now := time.Now()
	for _, tr := range store.Tiers[1:] {
		from := now.Add(-tr.DefaultRetention + time.Hour)
		if err := db.EnsureRange(ctx, tr, from, now); err != nil {
			return err
		}
		q := fmt.Sprintf(`INSERT INTO %s
			SELECT s.id, g, 1,
			 c, c*0.5, c*1.6, c*1.3,  m, m*0.9, m*1.1, m*1.05,  d, d, d, d,
			 r, r*0.2, r*3, r*2,  t, t*0.2, t*3, t*2
			FROM servers s, generate_series(date_trunc('minute', now()) - $1::interval, now(), $2::interval) g,
			LATERAL (SELECT 25 + 20*sin(extract(epoch FROM g)/86400*6.283 + length(s.name)) + 5*random() AS c,
			                50 + 10*sin(extract(epoch FROM g)/86400*6.283) + 3*random() AS m,
			                40 + (extract(epoch FROM g) - extract(epoch FROM now()))/3e7 AS d,
			                5e5 + 4e5*sin(extract(epoch FROM g)/43200) + 1e5*random() AS r,
			                2e5 + 1e5*sin(extract(epoch FROM g)/40000) + 5e4*random() AS t) v
			ON CONFLICT DO NOTHING`, tr.Table)
		if _, err := db.Pool.Exec(ctx, q, fmt.Sprintf("%d seconds", int64(tr.DefaultRetention.Seconds())-3600), fmt.Sprintf("%d seconds", int64(tr.Step.Seconds()))); err != nil {
			return err
		}
	}
	return nil
}

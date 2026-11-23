//go:build sim

package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"time"
)

// A simulated Windows host: the same canned approach as simcmds_sim.go, with the answers of the Windows modules
// (their data is not the data of the Linux ones). Switched on by SetDevPlatform("windows").

func init() {
	devPlatform = func(p string) {
		if p == "windows" {
			useSimWindows()
		}
	}
}

func useSimWindows() {
	ok := func(v any) CmdFunc {
		return func(context.Context, json.RawMessage, bool) (any, error) { return v, nil }
	}
	devOS = "Windows 11 Pro 24H2"
	devModules = []string{"metrics", "terminal", "desktop", "assistant", "services", "processes", "logs", "files", "disk", "users", "programs", "winupdate", "network", "system"}
	devCommands = map[string]CmdFunc{
		"services.list":      ok(simWinServices()),
		"processes.list":     ok(simProcesses()),
		"disk.state":         ok(simWinDisk()),
		"disk.scan":          ok(simWinScan()),
		"users.state":        ok(simWinUsers()),
		"apps.list":          ok(map[string]any{"apps": simWinApps()}),
		"winupdate.list":     ok(map[string]any{"updates": simWinUpdates(), "reboot_pending": true}),
		"network.state":      ok(simWinNet()),
		"assistant.sessions": simAssistSessions,
		"assistant.sas":      simSasToggle,
		"desktop.state":      simDesktopStateCmd,
		"desktop.enable":     simDesktopToggle(true),
		"desktop.disable":    simDesktopToggle(false),
	}
	devStreams = map[string]StreamFunc{"logs": simLogs, "assistant": fakeAssistStream(nil)}
}

func simWinServices() map[string]any {
	u := func(n, d, a, sub, e string) Unit {
		return Unit{Name: n, Description: d, Active: a, Sub: sub, Enabled: e}
	}
	return map[string]any{"services": []Unit{
		u("LinagerAgent", "Linager Agent", "active", "running", "enabled"),
		u("Dhcp", "DHCP Client", "active", "running", "enabled"),
		u("Dnscache", "DNS Client", "active", "running", "enabled"),
		u("EventLog", "Windows Event Log", "active", "running", "enabled"),
		u("MpsSvc", "Windows Defender Firewall", "active", "running", "enabled"),
		u("Spooler", "Print Spooler", "active", "running", "enabled"),
		u("TermService", "Remote Desktop Services", "active", "running", "enabled"),
		u("W32Time", "Windows Time", "inactive", "dead", "disabled"),
	}}
}

func simWinDisk() diskState {
	return diskState{Filesystems: []DiskFS{
		{Mount: "C:", Device: "C:", FS: "NTFS", Total: 256 << 30, Used: 142 << 30, Avail: 114 << 30, Pct: 55, PerDay: 400 << 20, DaysLeft: 290, HistoryDays: 14, Trend: []uint64{136 << 30, 138 << 30, 139 << 30, 140 << 30, 141 << 30, 142 << 30}},
		{Mount: "D:", Device: "D:", FS: "NTFS", Total: 1000 << 30, Used: 380 << 30, Avail: 620 << 30, Pct: 38, Trend: []uint64{}},
	}, Cleanup: []cleanItem{{Kind: "win-temp", Size: 2800 << 20, Frees: 2800 << 20, Count: 2}, {Kind: "win-update-cache", Size: 4100 << 20, Frees: 4100 << 20, Count: 1}, {Kind: "win-recycle", Size: 900 << 20, Frees: 900 << 20, Count: 1}, {Kind: "win-old", Size: 21000 << 20, Frees: 21000 << 20, Count: 1}}}
}

func simWinScan() diskScan {
	n := func(name string, size uint64, ch ...*DiskNode) *DiskNode {
		return &DiskNode{Name: name, Size: size << 20, Children: ch}
	}
	tree := n("C:", 142000, n("Windows", 28000, n("WinSxS", 9000), n("System32", 7000), n("Temp", 2800)), n("Program Files", 31000, n("Microsoft Office", 9000)), n("Users", 64000, n("Administrator", 52000)), n("Windows.old", 21000))
	return diskScan{At: time.Now().Unix(), Mount: "C:", Tree: tree, Categories: []DiskCategory{{ID: "sys", Size: 59000 << 20}, {ID: "home", Size: 64000 << 20}, {ID: "tmp", Size: 2800 << 20}, {ID: "other", Size: 16200 << 20}, {ID: "free", Size: 114000 << 20}}}
}

func simWinUsers() map[string]any {
	now := time.Now()
	rid := 1000
	u := func(name, full, desc string, enabled, builtin bool, ago time.Duration, groups ...string) map[string]any {
		rid++
		last := int64(0)
		if ago > 0 {
			last = now.Add(-ago).Unix()
		}
		return map[string]any{"name": name, "full_name": full, "description": desc, "enabled": enabled, "sid": fmt.Sprintf("S-1-5-21-1-2-3-%d", rid), "builtin": builtin,
			"last_logon": last, "pw_last_set": now.Add(-60 * 24 * time.Hour).Unix(), "pw_expires": 0, "groups": groups}
	}
	return map[string]any{"users": []map[string]any{
		u("Administrator", "", "Built-in account for administering the computer", true, true, 2*time.Hour, "Administrators"),
		u("alice", "Alice Example", "", true, false, 5*time.Hour, "Administrators", "Remote Desktop Users"),
		u("bob", "Bob Example", "Accounting", true, false, 30*time.Hour, "Users"),
		u("svc-backup", "", "Backup service account", true, false, 0, "Backup Operators"),
		u("Guest", "", "Built-in account for guest access to the computer", false, false, 0, "Guests"),
	}, "groups": []string{"Administrators", "Backup Operators", "Guests", "Remote Desktop Users", "Users"},
		"sessions": []map[string]any{{"id": 1, "user": "alice", "state": "Active", "type": "console"}, {"id": 2, "user": "bob", "state": "Disconnected", "type": "rdp"}}}
}

func simWinUpdates() []map[string]any {
	u := func(id, title, kb string, size int64, sev string, sec bool) map[string]any {
		return map[string]any{"id": id, "title": title, "kb": kb, "size": size, "severity": sev, "security": sec, "downloaded": sec, "need_reboot": sec}
	}
	return []map[string]any{
		u("0a1b2c3d-0000-0000-0000-000000000001", "2026-11 Cumulative Update for Windows 11 Version 24H2 for x64-based Systems", "KB5099001", 612<<20, "Critical", true),
		u("0a1b2c3d-0000-0000-0000-000000000002", "Windows Malicious Software Removal Tool x64 - v5.130", "KB890830", 52<<20, "Important", true),
		u("0a1b2c3d-0000-0000-0000-000000000003", ".NET 8.0 Security Update for x64", "KB5098002", 71<<20, "Important", true),
		u("0a1b2c3d-0000-0000-0000-000000000004", "Intel - Net - 22.250.1.2", "", 18<<20, "", false),
	}
}

func simWinNet() map[string]any {
	return map[string]any{"adapters": []map[string]any{
		{"index": 12, "name": "Ethernet", "description": "Intel(R) Ethernet Connection I219-V", "mac": "A4:BB:6D:12:34:56", "up": true, "virtual": false, "mtu": 1500, "speed_mbps": 1000, "dhcp": false, "ipv4": []string{"10.0.1.50/24"}, "gateway": "10.0.1.1", "dns": []string{"10.0.1.1", "1.1.1.1"}},
		{"index": 7, "name": "vEthernet (Default Switch)", "description": "Hyper-V Virtual Ethernet Adapter", "mac": "00:15:5D:AA:BB:01", "up": true, "virtual": true, "mtu": 1500, "speed_mbps": 10000, "dhcp": true, "ipv4": []string{"172.24.64.1/20"}, "gateway": "", "dns": []string{}},
		{"index": 3, "name": "Wi-Fi", "description": "Intel(R) Wi-Fi 6 AX201", "mac": "A4:BB:6D:12:34:57", "up": false, "virtual": false, "mtu": 1500, "speed_mbps": 0, "dhcp": true, "ipv4": []string{}, "gateway": "", "dns": []string{}},
	}, "pending": nil}
}

// simRDPOn is the state of the simulated remote desktop of the computer.
var simRDPOn atomic.Bool

func init() {
	simRDPOn.Store(true)
	devDesktopOff = func() bool { return !simRDPOn.Load() }
}

func simDesktopStateCmd(context.Context, json.RawMessage, bool) (any, error) {
	on := simRDPOn.Load()
	svc := "stopped"
	if on {
		svc = "running"
	}
	return map[string]any{"enabled": on, "listening": on, "port": 3389, "nla": true, "service": svc, "supported": true, "edition": "Windows 11 Pro"}, nil
}

func simDesktopToggle(on bool) CmdFunc {
	return func(ctx context.Context, p json.RawMessage, d bool) (any, error) {
		simRDPOn.Store(on)
		return simDesktopStateCmd(ctx, p, d)
	}
}

var simSasOn atomic.Bool

func simSasState() map[string]any {
	on := simSasOn.Load()
	v := 0
	if on {
		v = 1
	}
	return map[string]any{"policy": v, "enabled": on}
}

func simAssistSessions(context.Context, json.RawMessage, bool) (any, error) {
	return map[string]any{"sessions": []AssistSession{
		{ID: 1, Type: "console", User: "ACME\\i.petrova", State: "active", Console: true, Screen: true},
		{ID: 2, Type: "rdp", User: "ACME\\a.sidorov", State: "active", Client: "10.0.2.15", Screen: true},
		{ID: 3, Type: "rdp", User: "ACME\\m.kuznetsov", State: "disconnected", Client: "10.0.2.41"},
	}, "sas": simSasState()}, nil
}

func simSasToggle(_ context.Context, p json.RawMessage, _ bool) (any, error) {
	var in struct {
		Enable bool `json:"enable"`
	}
	_ = json.Unmarshal(p, &in)
	simSasOn.Store(in.Enable)
	return simSasState(), nil
}

func simWinApps() []AppInfo {
	return []AppInfo{
		{Name: "7-Zip 24.08 (x64)", Version: "24.08", Publisher: "Igor Pavlov", Installed: "2025-02-11", Size: 5_800_000, Scope: "machine", Arch: "x64"},
		{Name: "Google Chrome", Version: "131.0.6778.205", Publisher: "Google LLC", Installed: "2025-01-20", Size: 0, Scope: "machine", Arch: "x64"},
		{Name: "Microsoft Edge", Version: "131.0.2903.112", Publisher: "Microsoft Corporation", Installed: "2025-01-18", Size: 0, Scope: "machine", Arch: "x86"},
		{Name: "Notepad++ (64-bit x64)", Version: "8.7.4", Publisher: "Notepad++ Team", Installed: "2024-12-02", Size: 12_400_000, Scope: "machine", Arch: "x64"},
		{Name: "Linager Agent", Version: "0.9.0", Publisher: "Linager", Installed: "2025-03-01", Size: 18_300_000, Scope: "machine", Arch: "x64"},
		{Name: "Visual Studio Code", Version: "1.96.2", Publisher: "Microsoft Corporation", Installed: "2025-01-09", Size: 380_000_000, Scope: "user", Arch: ""},
	}
}

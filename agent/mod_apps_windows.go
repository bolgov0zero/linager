//go:build windows

package agent

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const uninstallPath = `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`

// appsListCmd is "apps.list": the programs that are installed for the whole computer (64 and 32 bit views) and for the
// profiles that are loaded now. It only reads the registry.
func appsListCmd(_ context.Context, _ json.RawMessage, _ bool) (any, error) {
	var raw []rawApp
	raw = append(raw, readUninstall(registry.LOCAL_MACHINE, uninstallPath, registry.WOW64_64KEY, "machine", "x64")...)
	raw = append(raw, readUninstall(registry.LOCAL_MACHINE, uninstallPath, registry.WOW64_32KEY, "machine", "x86")...)
	if users, err := registry.USERS.ReadSubKeyNames(-1); err == nil {
		for _, sid := range users {
			// only the real profiles: S-1-5-21-... and S-1-12-1-... (not the services, not the "_Classes" hives)
			if !strings.HasPrefix(sid, "S-1-5-21-") && !strings.HasPrefix(sid, "S-1-12-1-") || strings.HasSuffix(sid, "_Classes") {
				continue
			}
			raw = append(raw, readUninstall(registry.USERS, sid+`\`+uninstallPath, 0, "user", "")...)
		}
	}
	return map[string]any{"apps": normalizeApps(raw)}, nil
}

// readUninstall reads the Uninstall keys under root\path; a view that cannot be opened gives nothing.
func readUninstall(root registry.Key, path string, view uint32, scope, arch string) []rawApp {
	parent, err := registry.OpenKey(root, path, registry.ENUMERATE_SUB_KEYS|view)
	if err != nil {
		return nil
	}
	defer func() { _ = parent.Close() }()
	names, err := parent.ReadSubKeyNames(-1)
	if err != nil {
		return nil
	}
	var out []rawApp
	for _, name := range names {
		k, err := registry.OpenKey(parent, name, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		app := rawApp{
			Name:        regStr(k, "DisplayName"),
			Version:     regStr(k, "DisplayVersion"),
			Publisher:   regStr(k, "Publisher"),
			InstallDate: regStr(k, "InstallDate"),
			ParentKey:   regStr(k, "ParentKeyName"),
			ReleaseType: regStr(k, "ReleaseType"),
			Scope:       scope,
			Arch:        arch,
		}
		if n, _, err := k.GetIntegerValue("EstimatedSize"); err == nil {
			app.SizeKB = n
		}
		if n, _, err := k.GetIntegerValue("SystemComponent"); err == nil && n == 1 {
			app.SystemComponent = true
		}
		// the date is a number in some programs
		if app.InstallDate == "" {
			if n, _, err := k.GetIntegerValue("InstallDate"); err == nil {
				app.InstallDate = strconv.FormatUint(n, 10)
			}
		}
		_ = k.Close()
		out = append(out, app)
	}
	return out
}

func regStr(k registry.Key, name string) string {
	v, _, err := k.GetStringValue(name)
	if err != nil {
		return ""
	}
	return v
}

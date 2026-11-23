package agent

import (
	"sort"
	"strconv"
	"strings"
)

// AppInfo is one installed program of a Windows host, as the "Programs" tab shows it.
type AppInfo struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	Publisher string `json:"publisher"`
	// Installed is the install date as "YYYY-MM-DD", empty when the program did not say.
	Installed string `json:"installed"`
	// Size is the size on disk in bytes, 0 when the program did not say.
	Size int64 `json:"size"`
	// Scope is "machine" for a program of the whole computer and "user" for one of a single profile.
	Scope string `json:"scope"`
	// Arch is "x64" or "x86" for a program of the computer, empty for the profile ones.
	Arch string `json:"arch"`
}

// rawApp is what one Uninstall key of the registry says about a program.
type rawApp struct {
	Name            string
	Version         string
	Publisher       string
	InstallDate     string // "YYYYMMDD" as a string, or the same number written as text
	SizeKB          uint64 // EstimatedSize, in kilobytes
	SystemComponent bool
	ParentKey       string // ParentKeyName: set for an update of another program
	ReleaseType     string // "Update", "Security Update", "Hotfix"...
	Scope           string
	Arch            string
}

// normalizeApps turns the registry rows into the list: components and updates are left out, the same program seen through
// the 32 and 64 bit views is listed once, the order is by name.
func normalizeApps(raw []rawApp) []AppInfo {
	seen := map[string]bool{}
	out := make([]AppInfo, 0, len(raw))
	for _, r := range raw {
		name := strings.TrimSpace(r.Name)
		if name == "" || r.SystemComponent || r.ParentKey != "" || isUpdateRow(name, r.ReleaseType) {
			continue
		}
		key := strings.ToLower(name) + "\x00" + strings.TrimSpace(r.Version) + "\x00" + r.Scope
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, AppInfo{
			Name:      name,
			Version:   strings.TrimSpace(r.Version),
			Publisher: strings.TrimSpace(r.Publisher),
			Installed: formatInstallDate(r.InstallDate),
			Size:      int64(r.SizeKB) * 1024, //nolint:gosec // a registry DWORD of kilobytes cannot overflow int64 once multiplied
			Scope:     r.Scope,
			Arch:      r.Arch,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := strings.ToLower(out[i].Name), strings.ToLower(out[j].Name)
		if a != b {
			return a < b
		}
		return out[i].Version < out[j].Version
	})
	return out
}

// isUpdateRow is a Windows update or hotfix (they have their own place), for example "Update for Windows (KB5001234)".
func isUpdateRow(name, release string) bool {
	switch strings.ToLower(release) {
	case "update", "security update", "hotfix", "update rollup", "service pack":
		return true
	}
	n := strings.ToUpper(name)
	if strings.HasPrefix(n, "KB") && len(n) > 2 && n[2] >= '0' && n[2] <= '9' {
		return true
	}
	return strings.Contains(n, "(KB") && (strings.HasPrefix(n, "UPDATE FOR") || strings.HasPrefix(n, "SECURITY UPDATE FOR") || strings.HasPrefix(n, "HOTFIX"))
}

// formatInstallDate turns "20240131" into "2024-01-31"; anything else is "unknown".
func formatInstallDate(s string) string {
	s = strings.TrimSpace(s)
	if len(s) != 8 {
		return ""
	}
	y, errY := strconv.Atoi(s[:4])
	m, errM := strconv.Atoi(s[4:6])
	d, errD := strconv.Atoi(s[6:])
	if errY != nil || errM != nil || errD != nil || y < 1990 || y > 2200 || m < 1 || m > 12 || d < 1 || d > 31 {
		return ""
	}
	return s[:4] + "-" + s[4:6] + "-" + s[6:]
}

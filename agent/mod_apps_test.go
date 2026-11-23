package agent

import "testing"

func TestNormalizeApps(t *testing.T) {
	raw := []rawApp{
		{Name: "7-Zip", Version: "24.08", Publisher: " Igor Pavlov ", InstallDate: "20250211", SizeKB: 5000, Scope: "machine", Arch: "x64"},
		{Name: "7-Zip", Version: "24.08", Scope: "machine", Arch: "x86"}, // the same program through the 32 bit view
		{Name: "7-Zip", Version: "24.08", Scope: "user"},                 // another scope is another row
		{Name: "", Scope: "machine"},                                           // no name
		{Name: "Hidden component", SystemComponent: true, Scope: "machine"},    // a component
		{Name: "Patch", ParentKey: "Office", Scope: "machine"},                 // an update of another program
		{Name: "Security Update for Windows (KB5001234)", Scope: "machine"},    // an update by its name
		{Name: "KB5009999", Scope: "machine"},                                  // an update by its name
		{Name: "Some patch", ReleaseType: "Security Update", Scope: "machine"}, // an update by its type
		{Name: "alpha tool", Version: "1.0", InstallDate: "not a date", Scope: "machine"},
		{Name: "KBoard Pro", Version: "2", Scope: "machine"}, // starts with "KB" but is a program
	}
	got := normalizeApps(raw)
	var names []string
	for _, a := range got {
		names = append(names, a.Name+"/"+a.Scope)
	}
	want := []string{"7-Zip/machine", "7-Zip/user", "alpha tool/machine", "KBoard Pro/machine"}
	if len(names) != len(want) {
		t.Fatalf("got %v, want %v", names, want)
	}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("got %v, want %v", names, want)
		}
	}
	first := got[0]
	if first.Publisher != "Igor Pavlov" || first.Installed != "2025-02-11" || first.Size != 5000*1024 || first.Arch != "x64" {
		t.Errorf("bad fields: %+v", first)
	}
	if got[2].Installed != "" {
		t.Errorf("a broken date must be empty, got %q", got[2].Installed)
	}
}

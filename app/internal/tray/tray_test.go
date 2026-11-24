package tray

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreKeepsConnectionsPerProfile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "tray.json")
	s := Open(path)
	if err := s.Set("p1", Entry{ServerID: "a", Name: "web", Modes: []string{"asst", "terminal", "terminal"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("p1", Entry{ServerID: "b", Name: "Alpha", Modes: []string{"rdp"}}); err != nil {
		t.Fatal(err)
	}
	if err := s.Set("p2", Entry{ServerID: "c", Name: "other", Modes: []string{"terminal"}}); err != nil {
		t.Fatal(err)
	}
	l := s.List("p1")
	if len(l) != 2 || l[0].Name != "Alpha" || strings.Join(l[1].Modes, ",") != "terminal,asst" {
		t.Fatalf("sorted by name, modes in a fixed order without repeats: %+v", l)
	}
	if len(s.List("p2")) != 1 || len(s.List("none")) != 0 {
		t.Fatal("entries belong to the profile")
	}
	// it survives a restart
	if got := Open(path).List("p1"); len(got) != 2 {
		t.Fatalf("not kept: %+v", got)
	}
	// an empty list takes the server out
	if err := s.Set("p1", Entry{ServerID: "a", Name: "web"}); err != nil || len(s.List("p1")) != 1 {
		t.Fatalf("remove: %v %+v", err, s.List("p1"))
	}
	if s.Set("p1", Entry{ServerID: "a", Modes: []string{"ssh"}}) == nil || s.Set("", Entry{ServerID: "a"}) == nil || s.Set("p1", Entry{}) == nil {
		t.Fatal("an unknown connection, no profile and no server must be refused")
	}
	s.Rename("p1", "b", "Beta")
	if s.List("p1")[0].Name != "Beta" {
		t.Fatal("rename")
	}
}

func TestSummarizeUsesTheThresholdsOfTheDashboard(t *testing.T) {
	js := `{"items":[
	 {"online":true,"sample":{"cpu":10,"mem_pct":20,"disk_pct":30}},
	 {"online":true,"sample":{"cpu":80,"mem_pct":20,"disk_pct":30}},
	 {"online":true,"sample":{"cpu":10,"mem_pct":20,"disk_pct":30,"reboot_required":true}},
	 {"online":true,"sample":{"cpu":10,"mem_pct":20,"disk_pct":30,"updates":10}},
	 {"online":false,"sample":{"cpu":99}},
	 {"online":true}]}`
	st, err := Summarize([]byte(js))
	if err != nil || st.Total != 6 || st.Online != 5 || st.Attention != 3 {
		t.Fatalf("%+v %v", st, err)
	}
	if _, err := Summarize([]byte("x")); err == nil {
		t.Fatal("not json")
	}
}

func TestMenuHasOpenFirstAndQuitLast(t *testing.T) {
	es := []Entry{{ServerID: "a", Name: "web", Modes: []string{"terminal", "rdp"}}}
	m := Menu("ru", &Status{Total: 15, Online: 12, Attention: 2, Waiting: 1}, es)
	if m[0].Kind != "open" || m[0].Label != "Открыть" || m[len(m)-1].Kind != "quit" || m[len(m)-1].Label != "Закрыть" {
		t.Fatalf("first and last: %+v", m)
	}
	var conns []Item
	var status string
	for _, it := range m {
		if it.Kind == "conn" {
			conns = append(conns, it)
		}
		if it.Kind == "status" {
			status = it.Label
		}
	}
	if len(conns) != 2 || conns[0].Mode != "terminal" || conns[1].Mode != "rdp" || conns[0].Label != "web" {
		t.Fatalf("connections: %+v", conns)
	}
	if status != "В сети 12 из 15, внимание: 2, ждут: 1" {
		t.Fatalf("status: %q", status)
	}
	// not signed in: no state, no connections, but Open and Quit
	m = Menu("en", nil, nil)
	if len(m) != 3 || m[0].Label != "Open" || m[2].Label != "Quit" {
		t.Fatalf("empty menu: %+v", m)
	}
}

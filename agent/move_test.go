package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func moveState(t *testing.T) (string, *State) {
	t.Helper()
	dir := t.TempDir()
	st := &State{Server: "srv.example.com:9443", ServerFP: "SHA256:x"}
	if err := saveStateOnly(dir, st); err != nil {
		t.Fatal(err)
	}
	return dir, st
}

func TestStartMove(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	dir, st := moveState(t)
	for _, bad := range []int{0, -1, 70000, 9443} { // out of range, and the port in use now
		if ok, err := startMove(dir, st, bad, now); ok || err != nil || st.Next != "" {
			t.Fatalf("port %d must be ignored: %v %v %q", bad, ok, err, st.Next)
		}
	}
	if ok, err := startMove(dir, st, 9500, now); !ok || err != nil || st.Next != "srv.example.com:9500" {
		t.Fatalf("a new port: %v %v %q", ok, err, st.Next)
	}
	if ok, _ := startMove(dir, st, 9500, now); ok { // the same request again changes nothing
		t.Fatal("a repeated request must be ignored")
	}
	// the host never changes, only the port; the state is on disk
	var got State
	b, rerr := os.ReadFile(filepath.Join(dir, "state.json")) //nolint:gosec // a path inside the test directory
	if err := json.Unmarshal(b, &got); rerr != nil || err != nil || got.Next != "srv.example.com:9500" || got.Server != "srv.example.com:9443" {
		t.Fatalf("saved state: %+v %s", got, b)
	}
}

func TestMoveFallsBackAndRemembersTheBadPort(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	dir, st := moveState(t)
	_, _ = startMove(dir, st, 9500, now)
	for i := 1; i < maxMoveTries; i++ {
		if back, err := failMove(dir, st, now); back || err != nil || st.Next == "" {
			t.Fatalf("attempt %d must not give up yet: %v %v", i, back, err)
		}
	}
	if back, err := failMove(dir, st, now); !back || err != nil || st.Next != "" || st.Server != "srv.example.com:9443" || st.BadPort != 9500 {
		t.Fatalf("after %d failures the agent must go back: %v %v %+v", maxMoveTries, back, err, st)
	}
	if ok, _ := startMove(dir, st, 9500, now.Add(time.Minute)); ok {
		t.Fatal("the bad port must be ignored for a while")
	}
	if ok, _ := startMove(dir, st, 9500, now.Add(badPortFor+time.Second)); !ok {
		t.Fatal("the port may be tried again later")
	}
}

func TestPromoteMove(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	dir, st := moveState(t)
	_, _ = startMove(dir, st, 9500, now)
	if err := promoteMove(dir, st); err != nil || st.Server != "srv.example.com:9500" || st.Next != "" || st.NextTries != 0 || st.BadPort != 0 {
		t.Fatalf("promote: %v %+v", err, st)
	}
	if _, err := filepath.Glob(filepath.Join(dir, "state.json.tmp")); err != nil {
		t.Fatal(err)
	}
}

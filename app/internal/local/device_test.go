package local

import (
	"net/url"
	"strings"
	"testing"
)

func TestClientHeaderTellsWhatTheApplicationIs(t *testing.T) {
	q, err := url.ParseQuery(clientHeader("1.2.3"))
	if err != nil {
		t.Fatal(err)
	}
	if q.Get("app") != "1.2.3" || q.Get("platform") == "" || q.Get("platform") == "ios" {
		t.Fatalf("%v", q)
	}
	switch q.Get("platform") {
	case "macos", "windows", "linux", "other":
	default:
		t.Fatalf("platform %q", q.Get("platform"))
	}
}

func TestOsReleaseName(t *testing.T) {
	if got := osReleaseName("NAME=\"Ubuntu\"\nPRETTY_NAME=\"Ubuntu 24.04.1 LTS\"\nID=ubuntu\n"); got != "Ubuntu 24.04.1 LTS" {
		t.Fatal(got)
	}
	if osReleaseName("ID=x\n") != "" || strings.Contains(osReleaseName("PRETTY_NAME='A b'"), "'") {
		t.Fatal("quotes and missing")
	}
}

package autostart

import (
	"strings"
	"testing"
)

func TestPlistStartsABundleThroughLaunchServices(t *testing.T) {
	p := plist("/Applications/Linager.app/Contents/MacOS/linager-app")
	for _, want := range []string{"<string>/usr/bin/open</string>", "<string>/Applications/Linager.app</string>", "<string>-hidden</string>", "<key>RunAtLoad</key><true/>", ID} {
		if !strings.Contains(p, want) {
			t.Fatalf("missing %q in %s", want, p)
		}
	}
	// a loose program is started directly; special characters are escaped
	q := plist("/opt/a&b/linager-app")
	if !strings.Contains(q, "<string>/opt/a&amp;b/linager-app</string>") || strings.Contains(q, "/usr/bin/open") {
		t.Fatalf("loose program: %s", q)
	}
}

func TestDesktopFileQuotesThePath(t *testing.T) {
	if d := desktopFile("/usr/bin/linager-app"); !strings.Contains(d, "Exec=/usr/bin/linager-app -hidden\n") {
		t.Fatal(d)
	}
	if d := desktopFile("/home/a b/linager app"); !strings.Contains(d, `Exec="/home/a b/linager app" -hidden`) {
		t.Fatal(d)
	}
}

func TestRunValueQuotesThePath(t *testing.T) {
	if v := runValue(`C:\Program Files\Linager\linager-app.exe`); v != `"C:\Program Files\Linager\linager-app.exe" -hidden` {
		t.Fatal(v)
	}
}

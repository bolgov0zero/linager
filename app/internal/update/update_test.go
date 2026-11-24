package update

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/blake2b"
)

func TestVerifyRealMinisignFixture(t *testing.T) {
	pub, _ := os.ReadFile("testdata/release.pub")
	msg, _ := os.ReadFile("testdata/message.txt")
	sig, _ := os.ReadFile("testdata/message.txt.minisig")
	k, err := ParsePublicKey(string(pub))
	if err != nil {
		t.Fatal(err)
	}
	if err := k.Verify(msg, string(sig)); err != nil {
		t.Fatalf("a signature made by the minisign tool must verify: %v", err)
	}
	if k.Verify(append(msg, '!'), string(sig)) == nil {
		t.Fatal("a modified message must not verify")
	}
	if e, err := ParsePublicKey(PublicKey); err != nil || e.ID != k.ID {
		t.Fatal("the embedded key must be the release key used for the fixture")
	}
}

func TestVersionsAndChannels(t *testing.T) {
	if !Newer("v0.10.0-beta.4", "0.10.0-beta.3") || Newer("0.10.0-beta.3", "0.10.0-beta.3") || Newer("0.10.0-beta.2", "0.10.0-beta.3") {
		t.Fatal("betas must be compared by their number")
	}
	if !Newer("0.10.0", "0.10.0-beta.9") {
		t.Fatal("a stable version is above its betas")
	}
	if ChannelOf("0.10.0-beta.3") != "beta" || ChannelOf("1.0.0") != "stable" {
		t.Fatal("channel")
	}
}

// ---- a fake GitHub with signed releases ----

type fake struct {
	t     *testing.T
	pub   ed25519.PublicKey
	priv  ed25519.PrivateKey
	srv   *httptest.Server
	files map[string][]byte // path -> body
	rels  []map[string]any
	// the list of releases: an ETag is given, a request that has it gets 304, refuse makes every request answer this status
	listFull, list304, refuse int
	reset                     string
	// what the requests carried: the Authorization header of the list, and of every other request
	listAuth, otherAuth string
	badToken            bool
}

func sign(priv ed25519.PrivateKey, id [8]byte, msg []byte) string {
	h := blake2b.Sum512(msg)
	sig := ed25519.Sign(priv, h[:])
	raw := append(append([]byte("ED"), id[:]...), sig...)
	trusted := "timestamp:1"
	global := ed25519.Sign(priv, append(append([]byte(nil), sig...), trusted...))
	return "untrusted comment: test\n" + base64.StdEncoding.EncodeToString(raw) + "\ntrusted comment: " + trusted + "\n" + base64.StdEncoding.EncodeToString(global) + "\n"
}

func newFake(t *testing.T) *fake {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	f := &fake{t: t, pub: pub, priv: priv, files: map[string][]byte{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/rate_limit" {
			if f.badToken || r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte("{}"))
			return
		}
		if r.URL.Path != "/releases" {
			f.otherAuth += r.Header.Get("Authorization")
		}
		if r.URL.Path == "/releases" {
			f.listAuth = r.Header.Get("Authorization")
			if f.refuse != 0 {
				if f.reset != "" {
					w.Header().Set("X-RateLimit-Reset", f.reset)
				}
				w.WriteHeader(f.refuse)
				return
			}
			body, _ := json.Marshal(f.rels)
			tag := fmt.Sprintf(`"%x"`, sha256.Sum256(body))
			w.Header().Set("ETag", tag)
			if r.Header.Get("If-None-Match") == tag {
				f.list304++
				w.WriteHeader(http.StatusNotModified)
				return
			}
			f.listFull++
			_, _ = w.Write(body)
			return
		}
		b, ok := f.files[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(b)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *fake) updater(current, goos, goarch string) *Updater {
	k := &Key{Pub: f.pub}
	copy(k.ID[:], "testkey1")
	dir := f.t.TempDir()
	return &Updater{Current: current, API: f.srv.URL + "/releases", Client: f.srv.Client(), Key: k, GOOS: goos, GOARCH: goarch,
		Exe: func() (string, error) { return filepath.Join(dir, "x"), nil }, Launch: func(string) error { return nil }, Restart: func() {}, Dir: filepath.Join(dir, "dl"), AllowHTTP: true}
}

type relOpt struct {
	prerelease bool
	unsigned   bool
	badSig     bool
	wrongTag   bool   // the manifest names another version
	body       []byte // the archive; nil = no app asset
	lie        string // what the manifest says about the archive: "", "sha", "short", "long"
}

// release publishes a release with a signed manifest and one app asset for linux/amd64.
func (f *fake) release(tag string, o relOpt) {
	base := f.srv.URL + "/dl/" + tag + "/"
	assets := []map[string]string{}
	add := func(name string, b []byte) {
		f.files["/dl/"+tag+"/"+name] = b
		assets = append(assets, map[string]string{"name": name, "browser_download_url": base + name})
	}
	m := map[string]any{"version": strings.TrimPrefix(tag, "v"), "channel": "beta", "assets": map[string]Asset{}}
	if o.wrongTag {
		m["version"] = "9.9.9"
	}
	if o.body != nil {
		sum := sha256.Sum256(o.body)
		a := Asset{Name: "linager-desktop_linux_x64.tar.gz", SHA256: hex.EncodeToString(sum[:]), Size: int64(len(o.body))}
		switch o.lie {
		case "sha":
			a.SHA256 = strings.Repeat("0", 64)
		case "short":
			a.Size++
		case "long":
			a.Size--
		}
		m["assets"] = map[string]Asset{"app_linux_amd64": a}
		add(a.Name, o.body)
	}
	mb, _ := json.Marshal(m)
	if !o.unsigned {
		var id [8]byte
		copy(id[:], "testkey1")
		sig := sign(f.priv, id, mb)
		if o.badSig {
			mb2 := append([]byte(nil), mb...)
			mb2[len(mb2)-3] ^= 1
			add("manifest.json", mb2)
		} else {
			add("manifest.json", mb)
		}
		add("manifest.json.minisig", []byte(sig))
	}
	f.rels = append(f.rels, map[string]any{"tag_name": tag, "prerelease": o.prerelease, "body": "## Changes\n- one\n- two", "assets": assets})
}

func tarGz(t *testing.T, name string, body []byte) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	_, _ = tw.Write(body)
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestCheckOffersTheNewestSignedBuild(t *testing.T) {
	f := newFake(t)
	f.release("v0.10.0-beta.3", relOpt{prerelease: true, body: tarGz(t, "linager-app", []byte("old"))})
	f.release("v0.10.0-beta.4", relOpt{prerelease: true, body: tarGz(t, "linager-app", []byte("new"))})
	u := f.updater("0.10.0-beta.3", "linux", "amd64")
	info, err := u.Check(context.Background())
	if err != nil || !info.Newer || info.Latest != "0.10.0-beta.4" || info.Channel != "beta" || !info.Supported {
		t.Fatalf("got %+v, %v", info, err)
	}
	if len(info.Notes) != 2 || info.Notes[0] != "one" {
		t.Fatalf("notes: %v", info.Notes)
	}
	// the same version is not an update
	u = f.updater("0.10.0-beta.4", "linux", "amd64")
	if info, _ := u.Check(context.Background()); info.Newer {
		t.Fatal("the running version must not be offered again")
	}
}

func TestCheckNeverOffersWhatIsNotTrusted(t *testing.T) {
	cases := map[string]relOpt{
		"unsigned":      {prerelease: true, unsigned: true, body: tarGz(t, "linager-app", []byte("x"))},
		"bad signature": {prerelease: true, badSig: true, body: tarGz(t, "linager-app", []byte("x"))},
		"other version": {prerelease: true, wrongTag: true, body: tarGz(t, "linager-app", []byte("x"))},
		"no app build":  {prerelease: true},
	}
	for name, o := range cases {
		f := newFake(t)
		f.release("v0.10.0-beta.4", o)
		u := f.updater("0.10.0-beta.3", "linux", "amd64")
		info, _ := u.Check(context.Background())
		if info.Newer || info.Latest != "" {
			t.Fatalf("%s: offered %+v", name, info)
		}
	}
	// no build for this system
	f := newFake(t)
	f.release("v0.10.0-beta.4", relOpt{prerelease: true, body: tarGz(t, "linager-app", []byte("x"))})
	if info, _ := f.updater("0.10.0-beta.3", "darwin", "amd64").Check(context.Background()); info.Newer {
		t.Fatal("a build for another system must not be offered")
	}
}

func TestCheckFallsBackToAnOlderGoodRelease(t *testing.T) {
	f := newFake(t)
	f.release("v0.10.0-beta.4", relOpt{prerelease: true, body: tarGz(t, "linager-app", []byte("a"))})
	f.release("v0.10.0-beta.5", relOpt{prerelease: true, badSig: true, body: tarGz(t, "linager-app", []byte("b"))})
	info, err := f.updater("0.10.0-beta.3", "linux", "amd64").Check(context.Background())
	if err != nil || info.Latest != "0.10.0-beta.4" {
		t.Fatalf("got %+v, %v", info, err)
	}
}

func TestStableChannelSkipsBetas(t *testing.T) {
	f := newFake(t)
	f.release("v1.1.0-beta.1", relOpt{prerelease: true, body: tarGz(t, "linager-app", []byte("b"))})
	f.release("v1.0.1", relOpt{body: tarGz(t, "linager-app", []byte("s"))})
	info, _ := f.updater("1.0.0", "linux", "amd64").Check(context.Background())
	if info.Latest != "1.0.1" || info.Channel != "stable" {
		t.Fatalf("got %+v", info)
	}
	// a beta follows betas and newer stable versions
	info, _ = f.updater("1.0.0-beta.2", "linux", "amd64").Check(context.Background())
	if info.Latest != "1.1.0-beta.1" && info.Latest != "1.0.1" {
		t.Fatalf("got %+v", info)
	}
}

func TestDevAndHeadlessBuildsDoNotUpdate(t *testing.T) {
	f := newFake(t)
	f.release("v0.10.0-beta.4", relOpt{prerelease: true, body: tarGz(t, "linager-app", []byte("x"))})
	for _, v := range []string{"dev", "0.10.0-dev"} {
		u := f.updater(v, "linux", "amd64")
		info, _ := u.Check(context.Background())
		if info.Supported || info.Newer {
			t.Fatalf("%s: %+v", v, info)
		}
		if u.Install(context.Background()) == nil {
			t.Fatalf("%s: install must refuse", v)
		}
	}
	u := f.updater("0.10.0-beta.3", "linux", "amd64")
	u.Restart = nil
	if info, _ := u.Check(context.Background()); info.Supported || info.Reason != "headless" {
		t.Fatalf("headless: %+v", info)
	}
}

func TestInstallReplacesTheProgram(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	f := newFake(t)
	f.release("v0.10.0-beta.4", relOpt{prerelease: true, body: tarGz(t, "linager-app", []byte("NEW PROGRAM"))})
	u := f.updater("0.10.0-beta.3", "linux", "amd64")
	dir := t.TempDir()
	exe := filepath.Join(dir, "linager-app")
	if err := os.WriteFile(exe, []byte("OLD"), 0o755); err != nil {
		t.Fatal(err)
	}
	u.Exe = func() (string, error) { return exe, nil }
	var started string
	restarted := false
	u.Launch = func(p string) error { started = p; return nil }
	u.Restart = func() { restarted = true }
	if _, err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := u.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(exe)
	real, _ := filepath.EvalSymlinks(exe)
	if string(b) != "NEW PROGRAM" || started != real || !restarted {
		t.Fatalf("exe=%q started=%q restarted=%v", b, started, restarted)
	}
	if fi, _ := os.Stat(exe); fi.Mode().Perm()&0o100 == 0 {
		t.Fatal("the new program must be executable")
	}
	if st := u.Status(); st.Phase != "restarting" {
		t.Fatalf("status %+v", st)
	}
	es, _ := os.ReadDir(dir)
	if len(es) != 1 {
		t.Fatalf("nothing but the program must stay in its folder: %v", es)
	}
}

func TestInstallRefusesAFileThatDoesNotMatchTheManifest(t *testing.T) {
	for _, lie := range []string{"sha", "short", "long"} {
		f := newFake(t)
		f.release("v0.10.0-beta.4", relOpt{prerelease: true, lie: lie, body: tarGz(t, "linager-app", []byte("NEW"))})
		u := f.updater("0.10.0-beta.3", "linux", "amd64")
		dir := t.TempDir()
		exe := filepath.Join(dir, "linager-app")
		_ = os.WriteFile(exe, []byte("OLD"), 0o755)
		u.Exe = func() (string, error) { return exe, nil }
		if _, err := u.Check(context.Background()); err != nil {
			t.Fatal(err)
		}
		if err := u.Install(context.Background()); err == nil {
			t.Fatalf("%s: must fail", lie)
		}
		if b, _ := os.ReadFile(exe); string(b) != "OLD" {
			t.Fatalf("%s: the program was touched", lie)
		}
		if u.Status().Phase != "error" {
			t.Fatalf("%s: %+v", lie, u.Status())
		}
	}
}

func TestInstallWithoutACheckIsRefused(t *testing.T) {
	f := newFake(t)
	if err := f.updater("0.10.0-beta.3", "linux", "amd64").Install(context.Background()); err == nil {
		t.Fatal("nothing is offered yet")
	}
}

func TestHTTPIsRefusedInProduction(t *testing.T) {
	f := newFake(t)
	u := f.updater("0.10.0-beta.3", "linux", "amd64")
	u.AllowHTTP = false
	if _, err := u.Check(context.Background()); err == nil {
		t.Fatal("a plain http address must be refused")
	}
}

// ---- unpacking and swapping ----

func mkzip(t *testing.T, entries map[string]string, links map[string]string) string {
	p := filepath.Join(t.TempDir(), "a.zip")
	f, _ := os.Create(p)
	zw := zip.NewWriter(f)
	for n, c := range entries {
		w, _ := zw.Create(n)
		_, _ = w.Write([]byte(c))
	}
	for n, target := range links {
		h := &zip.FileHeader{Name: n}
		h.SetMode(os.ModeSymlink | 0o777)
		w, _ := zw.CreateHeader(h)
		_, _ = w.Write([]byte(target))
	}
	_ = zw.Close()
	_ = f.Close()
	return p
}

func TestUnzipRefusesPathsOutsideTheFolder(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "d")
	_ = os.MkdirAll(dest, 0o755)
	if err := unzipTo(mkzip(t, map[string]string{"../evil": "x"}, nil), dest); err == nil {
		t.Fatal("a ../ name must be refused")
	}
	if err := unzipTo(mkzip(t, map[string]string{"ok": "x"}, map[string]string{"l": "../../etc/passwd"}), dest); err == nil {
		t.Fatal("a link out of the folder must be refused")
	}
	if err := unzipTo(mkzip(t, map[string]string{"A.app/Contents/x": "1"}, map[string]string{"A.app/Contents/l": "x"}), dest); err != nil {
		t.Fatalf("a link inside must pass: %v", err)
	}
}

func TestSwapBundleKeepsTheOldOneWhenItFails(t *testing.T) {
	root := t.TempDir()
	bundle := filepath.Join(root, "Linager.app")
	_ = os.MkdirAll(filepath.Join(bundle, "Contents"), 0o755)
	_ = os.WriteFile(filepath.Join(bundle, "Contents", "v"), []byte("old"), 0o644)
	fresh := filepath.Join(root, "stage", "Linager.app")
	_ = os.MkdirAll(filepath.Join(fresh, "Contents"), 0o755)
	_ = os.WriteFile(filepath.Join(fresh, "Contents", "v"), []byte("new"), 0o644)
	if err := swapBundle(bundle, fresh); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(bundle, "Contents", "v")); string(b) != "new" {
		t.Fatal("the new bundle must stand")
	}
	if _, err := os.Stat(bundle + ".old"); err == nil {
		t.Fatal("the old bundle must be removed")
	}
	// a missing new bundle: the old one is put back
	if err := swapBundle(bundle, filepath.Join(root, "nothing")); err == nil {
		t.Fatal("must fail")
	}
	if b, _ := os.ReadFile(filepath.Join(bundle, "Contents", "v")); string(b) != "new" {
		t.Fatal("the bundle must be put back")
	}
}

func TestBundleOf(t *testing.T) {
	if b, err := bundleOf("/Applications/Linager.app/Contents/MacOS/linager-app"); err != nil || b != "/Applications/Linager.app" {
		t.Fatalf("%q %v", b, err)
	}
	if _, err := bundleOf("/usr/local/bin/linager-app"); err == nil {
		t.Fatal("a loose program is not a bundle")
	}
}

func TestWindowsSwapKeepsOldFileAndCleansItLater(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "linager-app.exe")
	_ = os.WriteFile(exe, []byte("OLD"), 0o755)
	arch := mkzip(t, map[string]string{"linager-app.exe": "NEW"}, nil)
	var started string
	if err := apply("windows", arch, exe, func(p string) error { started = p; return nil }); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "NEW" || started != exe {
		t.Fatalf("%q %q", b, started)
	}
	if b, _ := os.ReadFile(exe + ".old"); string(b) != "OLD" {
		t.Fatal("the old program is kept as .old until the next start")
	}
}

func TestDownloadStatusReachesOneHundred(t *testing.T) {
	f := newFake(t)
	f.release("v0.10.0-beta.4", relOpt{prerelease: true, body: tarGz(t, "linager-app", bytes.Repeat([]byte("z"), 200000))})
	u := f.updater("0.10.0-beta.3", "linux", "amd64")
	exe := filepath.Join(t.TempDir(), "linager-app")
	_ = os.WriteFile(exe, []byte("OLD"), 0o755)
	u.Exe = func() (string, error) { return exe, nil }
	if _, err := u.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := u.Install(context.Background()); err != nil {
		t.Fatal(err)
	}
	if st := u.Status(); st.Pct != 100 || st.Version != "0.10.0-beta.4" {
		t.Fatalf("%+v", st)
	}
}

func TestUnzipSkipsMacOSExtraFiles(t *testing.T) {
	dest := t.TempDir()
	if err := unzipTo(mkzip(t, map[string]string{"A.app/Contents/x": "1", "A.app/Contents/._x": "junk", "__MACOSX/A.app/x": "junk"}, nil), dest); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "A.app/Contents/._x")); err == nil {
		t.Fatal("an AppleDouble file must be skipped")
	}
	if _, err := os.Stat(filepath.Join(dest, "__MACOSX")); err == nil {
		t.Fatal("__MACOSX must be skipped")
	}
	if _, err := os.Stat(filepath.Join(dest, "A.app/Contents/x")); err != nil {
		t.Fatal("the real file must be there")
	}
}

func TestTheChosenChannelDecidesWhatIsOffered(t *testing.T) {
	f := newFake(t)
	f.release("v1.1.0-beta.1", relOpt{prerelease: true, body: tarGz(t, "linager-app", []byte("b"))})
	f.release("v1.0.1", relOpt{body: tarGz(t, "linager-app", []byte("s"))})
	// a stable version that follows the betas sees the beta
	u := f.updater("1.0.0", "linux", "amd64")
	u.SetChannel("beta")
	if info, _ := u.Check(context.Background()); info.Channel != "beta" || info.Latest != "1.1.0-beta.1" {
		t.Fatalf("beta channel: %+v", info)
	}
	// a beta that follows the stable ones sees none of the betas
	u = f.updater("1.0.0-beta.2", "linux", "amd64")
	u.SetChannel("stable")
	info, _ := u.Check(context.Background())
	if info.Channel != "stable" || info.Latest != "1.0.1" {
		t.Fatalf("stable channel: %+v", info)
	}
	if got := u.Info().Channel; got != "stable" {
		t.Fatalf("Info must report the chosen channel: %s", got)
	}
	// changing the channel drops what was offered: it belongs to the other one
	u.SetChannel("beta")
	if in := u.Info(); in.Newer || in.Channel != "beta" {
		t.Fatalf("after the change: %+v", in)
	}
	// anything else goes back to the channel of the running version
	u.SetChannel("")
	if got := u.Info().Channel; got != "beta" {
		t.Fatalf("follow the version: %s", got)
	}
}

// The list of releases is asked with its ETag: an unchanged list costs nothing of the limit of GitHub.
func TestTheListOfReleasesIsAskedWithItsETag(t *testing.T) {
	f := newFake(t)
	f.release("v0.10.0-beta.4", relOpt{prerelease: true, body: tarGz(t, "linager-app", []byte("new"))})
	u := f.updater("0.10.0-beta.3", "linux", "amd64")
	for i := 0; i < 3; i++ {
		info, err := u.Check(context.Background())
		if err != nil || !info.Newer || info.Latest != "0.10.0-beta.4" {
			t.Fatalf("check %d: %+v %v", i, info, err)
		}
	}
	if f.listFull != 1 || f.list304 != 2 {
		t.Fatalf("one full answer, then \"not modified\": %d and %d", f.listFull, f.list304)
	}
	// a new release changes the ETag: the new list is read
	f.release("v0.10.0-beta.5", relOpt{prerelease: true, body: tarGz(t, "linager-app", []byte("newer"))})
	if info, err := u.Check(context.Background()); err != nil || info.Latest != "0.10.0-beta.5" {
		t.Fatalf("after a new release: %+v %v", info, err)
	}
}

// When GitHub refuses for the number of requests, the error says so and when the address may ask again.
func TestACheckRefusedForTheLimitSaysSo(t *testing.T) {
	f := newFake(t)
	f.release("v0.10.0-beta.4", relOpt{prerelease: true, body: tarGz(t, "linager-app", []byte("new"))})
	f.refuse, f.reset = http.StatusForbidden, "1795540000"
	u := f.updater("0.10.0-beta.3", "linux", "amd64")
	_, err := u.Check(context.Background())
	var se *StatusError
	if !errors.As(err, &se) || !se.RateLimited() || se.Reset.Unix() != 1795540000 {
		t.Fatalf("a refusal for the limit: %v", err)
	}
	f.refuse = http.StatusInternalServerError
	if _, err := u.Check(context.Background()); !errors.As(err, &se) || se.RateLimited() {
		t.Fatalf("another refusal is not the limit: %v", err)
	}
}

// A GitHub token goes with the request of the list of releases only (that is what it is for), never with the downloads.
func TestTheTokenGoesWithTheListOnly(t *testing.T) {
	f := newFake(t)
	f.release("v0.10.0-beta.4", relOpt{prerelease: true, body: tarGz(t, "linager-app", []byte("new"))})
	u := f.updater("0.10.0-beta.3", "linux", "amd64")
	if _, err := u.Check(context.Background()); err != nil || f.listAuth != "" {
		t.Fatalf("without a token nothing is sent: %q %v", f.listAuth, err)
	}
	u.Token = func() string { return "ghp_secret" }
	if _, err := u.Check(context.Background()); err != nil || f.listAuth != "Bearer ghp_secret" {
		t.Fatalf("the list carries the token: %q %v", f.listAuth, err)
	}
	if f.otherAuth != "" {
		t.Fatalf("the manifest and the files go without it: %q", f.otherAuth)
	}
}

func TestVerifyTokenSaysWhenGitHubRefusesIt(t *testing.T) {
	f := newFake(t)
	u := f.updater("0.10.0-beta.3", "linux", "amd64")
	if err := u.VerifyToken(context.Background(), "good"); err != nil {
		t.Fatal(err)
	}
	f.badToken = true
	var se *StatusError
	if err := u.VerifyToken(context.Background(), "bad"); !errors.As(err, &se) || se.Code != http.StatusUnauthorized {
		t.Fatalf("a refused token: %v", err)
	}
}

package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bolgov0zero/linager/internal/release"
)

// publishArches adds a release with one binary per architecture. The top-level checksum is the amd64
// one, as in real manifests. tamper names an architecture whose served file differs from the signed sum.
func (r *fakeRepo) publishArches(tag string, bins map[string]string, tamper string, skipAsset string) {
	assets := map[string]release.Asset{}
	list := []map[string]string{}
	u := func(n string) string { return r.srv.URL + "/dl/" + tag + "/" + n }
	for arch, body := range bins {
		sum := sha256.Sum256([]byte(body))
		a := release.Asset{Name: release.BinaryAsset(arch), SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body))}
		assets[arch] = a
		served := body
		if arch == tamper {
			served += "evil"
		}
		r.data[tag+"/"+a.Name] = []byte(served)
		if arch != skipAsset {
			list = append(list, map[string]string{"name": a.Name, "browser_download_url": u(a.Name)})
		}
	}
	top := assets[release.ArchAMD64]
	m, _ := json.Marshal(release.Manifest{Version: tag, Channel: "stable", MinAgent: "v0.1.0", SHA256: top.SHA256, Size: top.Size, Assets: assets})
	r.data[tag+"/m"], r.data[tag+"/s"] = m, []byte(signFile(r.priv, r.key.ID, m))
	list = append(list, map[string]string{"name": "manifest.json", "browser_download_url": u("m")}, map[string]string{"name": "manifest.json.minisig", "browser_download_url": u("s")})
	r.rels = append(r.rels, map[string]any{"tag_name": tag, "assets": list})
}

func withHostArch(t *testing.T, arch string) {
	t.Helper()
	old := release.HostArch
	release.HostArch = arch
	t.Cleanup(func() { release.HostArch = old })
}

func TestDownloadStoresEveryArchitecture(t *testing.T) {
	r := newFakeRepo(t)
	r.publishArches("v0.2.0", map[string]string{"amd64": "AMD64-binary", "arm64": "ARM64-binary-longer"}, "", "")
	rels, _ := URL(r.srv.URL).Releases(context.Background())
	dir := t.TempDir()
	c, err := Download(context.Background(), rels[0], r.key, dir)
	if err != nil {
		t.Fatal(err)
	}
	for arch, want := range map[string]string{"amd64": "AMD64-binary", "arm64": "ARM64-binary-longer"} {
		b, err := os.ReadFile(c.BinaryPathFor(arch))
		if err != nil || string(b) != want {
			t.Fatalf("%s: %q %v", arch, b, err)
		}
		if st, _ := os.Stat(c.BinaryPathFor(arch)); st.Mode().Perm()&0o100 == 0 {
			t.Fatalf("%s binary must be executable", arch)
		}
	}
	// Cached.BinaryPath and Size follow the architecture of the running server
	withHostArch(t, release.ArchARM64)
	l, err := LoadCached(r.key, dir, "v0.2.0")
	if err != nil {
		t.Fatal(err)
	}
	if l.BinaryPath() != l.BinaryPathFor("arm64") || l.Size != int64(len("ARM64-binary-longer")) || l.Asset().Name != "linager_linux_arm64" {
		t.Fatalf("%+v %s", l, l.BinaryPath())
	}
}

func TestDownloadRejectsATamperedForeignArchitecture(t *testing.T) {
	r := newFakeRepo(t)
	// the running (amd64) server would never execute the arm64 file, but it serves it to agents
	r.publishArches("v0.2.0", map[string]string{"amd64": "good", "arm64": "good-too"}, "arm64", "")
	rels, _ := URL(r.srv.URL).Releases(context.Background())
	dir := t.TempDir()
	if _, err := Download(context.Background(), rels[0], r.key, dir); err == nil || !strings.Contains(err.Error(), "checksum") {
		t.Fatalf("got %v", err)
	}
	if ents, _ := os.ReadDir(dir); len(ents) != 0 {
		t.Fatal("nothing may be cached after a failed download")
	}
}

func TestDownloadNeedsEveryDeclaredAsset(t *testing.T) {
	r := newFakeRepo(t)
	r.publishArches("v0.2.0", map[string]string{"amd64": "a", "arm64": "b"}, "", "arm64") // arm64 file missing from the release
	rels, _ := URL(r.srv.URL).Releases(context.Background())
	if _, err := Download(context.Background(), rels[0], r.key, t.TempDir()); err == nil || !strings.Contains(err.Error(), "linager_linux_arm64") {
		t.Fatalf("got %v", err)
	}
}

func TestDownloadRefusesAReleaseWithoutTheHostBinary(t *testing.T) {
	r := newFakeRepo(t)
	r.publish("v0.1.0", false, "legacy amd64 only", false) // no assets map
	rels, _ := URL(r.srv.URL).Releases(context.Background())
	withHostArch(t, release.ArchARM64)
	if _, err := Download(context.Background(), rels[0], r.key, t.TempDir()); err == nil || !strings.Contains(err.Error(), "arm64") {
		t.Fatalf("got %v", err)
	}
}

func TestLoadCachedChecksEveryArchitecture(t *testing.T) {
	r := newFakeRepo(t)
	r.publishArches("v0.2.0", map[string]string{"amd64": "aaa", "arm64": "bbb"}, "", "")
	rels, _ := URL(r.srv.URL).Releases(context.Background())
	dir := t.TempDir()
	c, err := Download(context.Background(), rels[0], r.key, dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(c.BinaryPathFor("arm64"), []byte("evil"), 0o755) //nolint:gosec // test fixture
	if _, err := LoadCached(r.key, dir, "v0.2.0"); err == nil {
		t.Fatal("a modified binary of another architecture must be rejected: it is served to agents")
	}
	_ = os.WriteFile(c.BinaryPathFor("arm64"), []byte("bbb"), 0o755) //nolint:gosec // test fixture
	_ = os.Remove(c.BinaryPathFor("arm64"))
	if _, err := LoadCached(r.key, dir, "v0.2.0"); err == nil {
		t.Fatal("a missing binary must be rejected")
	}
}

// publishWindows adds the Windows agent to a release that was published with publishArches. tamper serves a file
// that differs from the signed checksum.
func (r *fakeRepo) publishWindows(tag, body string, tamper bool) {
	r.publishWindowsNamed(tag, release.WindowsBinaryAsset, body, tamper)
}

// publishWindowsNamed is publishWindows with the file name of the Windows agent chosen by the release.
func (r *fakeRepo) publishWindowsNamed(tag, name, body string, tamper bool) {
	var rel map[string]any
	for _, x := range r.rels {
		if x["tag_name"] == tag {
			rel = x
		}
	}
	var m release.Manifest
	_ = json.Unmarshal(r.data[tag+"/m"], &m)
	sum := sha256.Sum256([]byte(body))
	m.Assets[release.WindowsKey] = release.Asset{Name: name, SHA256: hex.EncodeToString(sum[:]), Size: int64(len(body))}
	mb, _ := json.Marshal(m)
	r.data[tag+"/m"], r.data[tag+"/s"] = mb, []byte(signFile(r.priv, r.key.ID, mb))
	served := body
	if tamper {
		served += "evil"
	}
	r.data[tag+"/"+name] = []byte(served)
	rel["assets"] = append(rel["assets"].([]map[string]string), map[string]string{"name": name, "browser_download_url": r.srv.URL + "/dl/" + tag + "/" + name})
}

func TestDownloadStoresTheWindowsAgent(t *testing.T) {
	r := newFakeRepo(t)
	r.publishArches("v0.2.0", map[string]string{"amd64": "AMD64-binary", "arm64": "ARM64-binary"}, "", "")
	r.publishWindows("v0.2.0", "WINDOWS-exe", false)
	rels, _ := URL(r.srv.URL).Releases(context.Background())
	dir := t.TempDir()
	c, err := Download(context.Background(), rels[0], r.key, dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(c.Dir + "/" + release.WindowsBinaryAsset); err != nil || string(b) != "WINDOWS-exe" {
		t.Fatalf("the Windows file is not in the cache: %q %v", b, err)
	}
	if _, ok := c.Manifest.WindowsAsset(); !ok {
		t.Fatal("the manifest must carry the Windows asset")
	}
	// the architectures that servers know are not confused by the Windows key
	if got := len(c.Manifest.AssetList()); got != 2 {
		t.Fatalf("AssetList must list the Linux architectures only, got %d", got)
	}
	if _, err := LoadCached(r.key, dir, "v0.2.0"); err != nil {
		t.Fatal(err)
	}
	// a file that no longer matches its signed checksum is not distributed
	if err := os.WriteFile(c.Dir+"/"+release.WindowsBinaryAsset, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadCached(r.key, dir, "v0.2.0"); err == nil {
		t.Fatal("a tampered Windows file must fail the check")
	}
}

// The file names of a release come from its signed manifest, so a release may rename its files (the Windows agent was
// "linager_windows_amd64.exe" before it became "linager-agent_windows_amd64.exe") and servers still fetch and serve them.
func TestDownloadFollowsTheNamesOfTheManifest(t *testing.T) {
	for _, name := range []string{"linager_windows_amd64.exe", "linager-agent_windows_amd64.exe", "anything-else.exe"} {
		r := newFakeRepo(t)
		r.publishArches("v0.2.0", map[string]string{"amd64": "AMD64-binary", "arm64": "ARM64-binary"}, "", "")
		r.publishWindowsNamed("v0.2.0", name, "WINDOWS-exe", false)
		rels, _ := URL(r.srv.URL).Releases(context.Background())
		dir := t.TempDir()
		c, err := Download(context.Background(), rels[0], r.key, dir)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		b, err := os.ReadFile(filepath.Join(c.Dir, name)) //nolint:gosec // a test reads a file of its own cache
		if err != nil || string(b) != "WINDOWS-exe" {
			t.Fatalf("%s is not in the cache: %q %v", name, b, err)
		}
		if _, err := LoadCached(r.key, dir, "v0.2.0"); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
	}
}

func TestDownloadRefusesATamperedWindowsAgent(t *testing.T) {
	r := newFakeRepo(t)
	r.publishArches("v0.2.0", map[string]string{"amd64": "AMD64-binary"}, "", "")
	r.publishWindows("v0.2.0", "WINDOWS-exe", true)
	rels, _ := URL(r.srv.URL).Releases(context.Background())
	if _, err := Download(context.Background(), rels[0], r.key, t.TempDir()); err == nil || !strings.Contains(err.Error(), release.WindowsBinaryAsset) {
		t.Fatalf("a tampered Windows file must be refused: %v", err)
	}
}

func TestReleaseWithoutTheWindowsFileStillServesLinux(t *testing.T) {
	r := newFakeRepo(t)
	r.publishArches("v0.2.0", map[string]string{"amd64": "AMD64-binary"}, "", "")
	rels, _ := URL(r.srv.URL).Releases(context.Background())
	c, err := Download(context.Background(), rels[0], r.key, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := c.Manifest.WindowsAsset(); ok {
		t.Fatal("this release has no Windows file")
	}
}

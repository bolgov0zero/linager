package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	neturl "net/url"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DefaultAPI is where the releases of the project are listed.
const DefaultAPI = "https://api.github.com/repos/bolgov0zero/linager/releases?per_page=30"

const (
	maxManifest = 1 << 20
	maxSig      = 4096
	maxArchive  = 400 << 20
)

// Asset is one file of a release as the signed manifest describes it.
type Asset struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

type manifest struct {
	Version string           `json:"version"`
	Channel string           `json:"channel"`
	Assets  map[string]Asset `json:"assets"`
}

// Release is one published release.
type Release struct {
	Tag        string
	Prerelease bool
	Notes      string
	Assets     map[string]string // asset name -> download URL
}

// Info is what a check found.
type Info struct {
	// Supported: this build can replace itself (a build of a release can, a build from a working tree cannot).
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
	Current   string `json:"current"`
	Channel   string `json:"channel"`
	// Latest is the newest version that has a build for this system; Newer says it is above the running one.
	Latest    string   `json:"latest,omitempty"`
	Newer     bool     `json:"newer"`
	Notes     []string `json:"notes,omitempty"`
	Size      int64    `json:"size,omitempty"`
	CheckedAt int64    `json:"checked_at,omitempty"`
}

// StatusError is an answer of a server that is not 200. Reset is when GitHub says the request limit of this address is
// renewed (zero when it does not say).
type StatusError struct {
	URL    string
	Code   int
	Status string
	Reset  time.Time
}

func (e *StatusError) Error() string { return e.URL + ": " + e.Status }

// RateLimited says whether GitHub refused for the number of requests (an address may ask 60 times an hour without signing in).
func (e *StatusError) RateLimited() bool {
	return e.Code == http.StatusTooManyRequests || e.Code == http.StatusForbidden
}

// retryAfter reads when a refused request may be repeated: X-RateLimit-Reset (a unix time) or Retry-After (seconds).
func retryAfter(h http.Header, now time.Time) time.Time {
	if v, err := strconv.ParseInt(h.Get("X-RateLimit-Reset"), 10, 64); err == nil && v > 0 {
		return time.Unix(v, 0)
	}
	if v, err := strconv.Atoi(h.Get("Retry-After")); err == nil && v > 0 {
		return now.Add(time.Duration(v) * time.Second)
	}
	return time.Time{}
}

// Status is the progress of an installation.
type Status struct {
	// Phase: idle, downloading, installing, restarting or error.
	Phase   string `json:"phase"`
	Pct     int    `json:"pct"`
	Version string `json:"version,omitempty"`
	Error   string `json:"error,omitempty"`
}

// Updater checks for a new version and installs it.
type Updater struct {
	Current string
	// API is the address of the release list (GitHub's shape); tests point it at their own server.
	API    string
	Client *http.Client
	Key    *Key
	GOOS   string
	GOARCH string
	// Exe is the path of the running program.
	Exe func() (string, error)
	// Launch starts the installed program; Restart ends this one afterwards.
	Launch  func(path string) error
	Restart func()
	// Dir keeps what is downloaded.
	Dir string
	// AllowHTTP lets a test serve the files over plain http.
	AllowHTTP bool
	// Token returns the GitHub token the person gave (empty: none). It is sent with the request of the list of releases only,
	// so that the request counts against the limit of the token (5000 an hour) and not against the one of the address (60).
	Token func() string

	// channel is the one the person chose ("stable" or "beta"); empty follows the running version.
	channel string

	mu     sync.Mutex
	info   Info
	rel    Release
	asset  Asset
	status Status
	busy   bool
}

// New is an updater of the running program: current is its version, dir keeps the downloads, restart ends the program.
func New(current, dir string, restart func()) (*Updater, error) {
	key, err := ParsePublicKey(PublicKey)
	if err != nil {
		return nil, err
	}
	return &Updater{Current: current, API: DefaultAPI, Client: &http.Client{Timeout: 30 * time.Second}, Key: key, GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
		Exe: os.Executable, Launch: launch, Restart: restart, Dir: dir}, nil
}

// AssetKey is the key of the build for a system in the manifest ("app_darwin_arm64").
func AssetKey(goos, goarch string) string { return "app_" + goos + "_" + goarch }

// ChannelOf is the channel a version follows: a beta follows the betas, a stable version the stable ones.
func ChannelOf(version string) string {
	if v := ParseVersion(version); v.Valid && v.Pre == "" {
		return "stable"
	}
	return "beta"
}

// SetChannel chooses the channel to follow ("stable" or "beta"); anything else goes back to the one of the running version.
func (u *Updater) SetChannel(c string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if c != "stable" && c != "beta" {
		c = ""
	}
	if c == u.channel {
		return
	}
	u.channel = c
	// what was offered belongs to the other channel
	u.info, u.rel, u.asset = Info{}, Release{}, Asset{}
}

func (u *Updater) channelLocked() string {
	if u.channel != "" {
		return u.channel
	}
	return ChannelOf(u.Current)
}

// followed is the channel in use: the chosen one, or the one of the running version.
func (u *Updater) followed() string {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.channelLocked()
}

// supported says whether a build can update itself; a build from a working tree (version "dev" or "...-dev") cannot.
func (u *Updater) supported() (bool, string) {
	v := ParseVersion(u.Current)
	if !v.Valid || strings.Contains(v.Pre, "dev") {
		return false, "dev"
	}
	if u.Restart == nil {
		return false, "headless"
	}
	return true, ""
}

// Info returns what the last check found.
func (u *Updater) Info() Info {
	u.mu.Lock()
	defer u.mu.Unlock()
	if u.info.Current == "" {
		// nothing checked yet: what is known without asking
		ok, why := u.supported()
		return Info{Supported: ok, Reason: why, Current: u.Current, Channel: u.channelLocked()}
	}
	in := u.info
	in.Channel = u.channelLocked()
	return in
}

// Status returns the progress of the installation.
func (u *Updater) Status() Status {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.status
}

// VerifyToken asks GitHub whether a token is accepted (the request of the limit itself costs nothing). A refused token is a
// *StatusError with the code 401.
func (u *Updater) VerifyToken(ctx context.Context, token string) error {
	pu, err := neturl.Parse(u.API)
	if err != nil || pu.Host == "" {
		return errors.New("bad address of the release list")
	}
	url := pu.Scheme + "://" + pu.Host + "/rate_limit"
	if !strings.HasPrefix(url, "https://") && !(u.AllowHTTP && strings.HasPrefix(url, "http://")) {
		return errors.New("only https addresses are used")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "linager-app-updater")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := u.Client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return &StatusError{URL: url, Code: resp.StatusCode, Status: resp.Status, Reset: retryAfter(resp.Header, time.Now())}
	}
	return nil
}

func (u *Updater) get(ctx context.Context, url string, limit int64) ([]byte, error) {
	resp, err := u.fetch(ctx, url)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("answer too big")
	}
	return b, nil
}

func (u *Updater) fetch(ctx context.Context, url string) (*http.Response, error) {
	return u.fetchTag(ctx, url, "")
}

// fetchTag is fetch with an ETag the caller already has: 304 (not changed) is answered as it is, and GitHub does not count
// such a request against the limit.
func (u *Updater) fetchTag(ctx context.Context, url, etag string) (*http.Response, error) {
	if !strings.HasPrefix(url, "https://") && !(u.AllowHTTP && strings.HasPrefix(url, "http://")) {
		return nil, errors.New("only https addresses are used")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "linager-app-updater")
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	if u.Token != nil && url == u.API {
		if tok := u.Token(); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
	}
	resp, err := u.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode == http.StatusNotModified && etag != "" {
		return resp, nil
	}
	if resp.StatusCode != http.StatusOK {
		_ = resp.Body.Close()
		return nil, &StatusError{URL: url, Code: resp.StatusCode, Status: resp.Status, Reset: retryAfter(resp.Header, time.Now())}
	}
	return resp, nil
}

type ghRelease struct {
	Tag        string `json:"tag_name"`
	Prerelease bool   `json:"prerelease"`
	Draft      bool   `json:"draft"`
	Body       string `json:"body"`
	Assets     []struct {
		Name string `json:"name"`
		URL  string `json:"browser_download_url"`
	} `json:"assets"`
}

// releasesCache is the last list of releases with its ETag: asking again with the ETag costs nothing of the request limit.
type releasesCache struct {
	ETag string `json:"etag"`
	Body []byte `json:"body"`
}

func (u *Updater) cachePath() string { return filepath.Join(u.Dir, "releases-cache.json") }

func (u *Updater) loadCache() releasesCache {
	var c releasesCache
	if b, err := os.ReadFile(u.cachePath()); err == nil { //nolint:gosec // the application's own file
		_ = json.Unmarshal(b, &c)
	}
	return c
}

func (u *Updater) saveCache(c releasesCache) {
	if err := os.MkdirAll(u.Dir, 0o700); err != nil {
		return
	}
	if b, err := json.Marshal(c); err == nil {
		_ = os.WriteFile(u.cachePath(), b, 0o600)
	}
}

// listBody reads the list of releases; a "not modified" answer gives the cached list.
func (u *Updater) listBody(ctx context.Context) ([]byte, error) {
	c := u.loadCache()
	if len(c.Body) == 0 {
		c.ETag = ""
	}
	resp, err := u.fetchTag(ctx, u.API, c.ETag)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotModified {
		return c.Body, nil
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20+1))
	if err != nil {
		return nil, err
	}
	if len(b) > 8<<20 {
		return nil, errors.New("answer too big")
	}
	if tag := resp.Header.Get("ETag"); tag != "" {
		u.saveCache(releasesCache{ETag: tag, Body: b})
	}
	return b, nil
}

func (u *Updater) releases(ctx context.Context) ([]Release, error) {
	b, err := u.listBody(ctx)
	if err != nil {
		return nil, err
	}
	var raw []ghRelease
	if err := json.Unmarshal(b, &raw); err != nil {
		return nil, err
	}
	var out []Release
	for _, g := range raw {
		if g.Draft {
			continue
		}
		r := Release{Tag: g.Tag, Prerelease: g.Prerelease, Notes: g.Body, Assets: map[string]string{}}
		for _, a := range g.Assets {
			r.Assets[a.Name] = a.URL
		}
		out = append(out, r)
	}
	return out, nil
}

// verifiedManifest reads the manifest of a release and checks the signature and that it is the manifest of this very release.
func (u *Updater) verifiedManifest(ctx context.Context, r Release) (*manifest, error) {
	mu, su := r.Assets["manifest.json"], r.Assets["manifest.json.minisig"]
	if mu == "" || su == "" {
		return nil, errors.New("the release is not signed")
	}
	mb, err := u.get(ctx, mu, maxManifest)
	if err != nil {
		return nil, err
	}
	sb, err := u.get(ctx, su, maxSig)
	if err != nil {
		return nil, err
	}
	if err := u.Key.Verify(mb, string(sb)); err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(mb, &m); err != nil {
		return nil, err
	}
	if strings.TrimPrefix(m.Version, "v") != strings.TrimPrefix(r.Tag, "v") {
		return nil, errors.New("the manifest is of another version")
	}
	return &m, nil
}

// notes are the lines of the release notes without the list marks.
func notes(body string) []string {
	var out []string
	for _, l := range strings.Split(body, "\n") {
		l = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(l), "-*"))
		if l != "" && !strings.HasPrefix(l, "#") {
			out = append(out, l)
		}
		if len(out) >= 15 {
			break
		}
	}
	return out
}

// Check reads the list of releases, takes the newest one of the channel that has a build for this system, and checks
// its signed manifest. A release that is not signed, or whose manifest does not match, is never offered.
func (u *Updater) Check(ctx context.Context) (Info, error) {
	ok, why := u.supported()
	info := Info{Supported: ok, Reason: why, Current: u.Current, Channel: u.followed(), CheckedAt: time.Now().Unix()}
	if !ok {
		// a build that cannot replace itself has nothing to look for
		u.mu.Lock()
		u.info = info
		u.mu.Unlock()
		return info, nil
	}
	rels, err := u.releases(ctx)
	if err != nil {
		return info, err
	}
	sort.Slice(rels, func(i, j int) bool { return Compare(ParseVersion(rels[i].Tag), ParseVersion(rels[j].Tag)) > 0 })
	key := AssetKey(u.GOOS, u.GOARCH)
	tried := 0
	for _, r := range rels {
		v := ParseVersion(r.Tag)
		if !v.Valid || (info.Channel == "stable" && (v.Pre != "" || r.Prerelease)) {
			continue
		}
		// nothing at or below the running version is looked at: newer ones only, the newest first
		if !Newer(r.Tag, u.Current) {
			break
		}
		if tried++; tried > 5 {
			break
		}
		m, merr := u.verifiedManifest(ctx, r)
		if merr != nil {
			err = merr
			continue
		}
		a, has := m.Assets[key]
		if !has || a.Name == "" || a.SHA256 == "" || a.Size <= 0 || a.Size > maxArchive || r.Assets[a.Name] == "" {
			continue
		}
		info.Latest, info.Newer, info.Notes, info.Size = strings.TrimPrefix(r.Tag, "v"), true, notes(r.Notes), a.Size
		u.mu.Lock()
		u.info, u.rel, u.asset = info, r, a
		u.mu.Unlock()
		return info, nil
	}
	u.mu.Lock()
	u.info, u.rel, u.asset = info, Release{}, Asset{}
	u.mu.Unlock()
	if info.Latest == "" && err != nil && tried > 0 {
		return info, err
	}
	return info, nil
}

func (u *Updater) setStatus(s Status) {
	u.mu.Lock()
	u.status = s
	u.mu.Unlock()
}

// Install downloads the version the last check found, verifies it against the signed manifest, replaces the running
// program and restarts it. It reports progress through Status; an error is kept there too.
func (u *Updater) Install(ctx context.Context) error {
	u.mu.Lock()
	if u.busy {
		u.mu.Unlock()
		return errors.New("an update is already running")
	}
	rel, a, info := u.rel, u.asset, u.info
	if ok, why := u.supported(); !ok || !info.Newer || a.Name == "" {
		u.mu.Unlock()
		if !ok {
			return errors.New("this build cannot update itself: " + why)
		}
		return errors.New("no update is offered")
	}
	u.busy = true
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.busy = false
		u.mu.Unlock()
	}()
	fail := func(err error) error {
		u.setStatus(Status{Phase: "error", Version: info.Latest, Error: err.Error()})
		return err
	}

	if err := os.MkdirAll(u.Dir, 0o700); err != nil {
		return fail(err)
	}
	// what an earlier try left is dropped
	if es, err := os.ReadDir(u.Dir); err == nil {
		for _, e := range es {
			_ = os.RemoveAll(filepath.Join(u.Dir, e.Name()))
		}
	}
	archive := filepath.Join(u.Dir, filepath.Base(a.Name))
	u.setStatus(Status{Phase: "downloading", Version: info.Latest})
	if err := u.download(ctx, rel.Assets[a.Name], a, archive); err != nil {
		_ = os.Remove(archive)
		return fail(err)
	}
	u.setStatus(Status{Phase: "installing", Pct: 100, Version: info.Latest})
	exe, err := u.Exe()
	if err != nil {
		return fail(err)
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return fail(err)
	}
	err = apply(u.GOOS, archive, exe, u.Launch)
	_ = os.Remove(archive)
	if err != nil {
		return fail(err)
	}
	u.setStatus(Status{Phase: "restarting", Pct: 100, Version: info.Latest})
	if u.Restart != nil {
		u.Restart()
	}
	return nil
}

// download fetches a file, counting the bytes, and keeps it only if its size and sha256 are what the manifest says.
func (u *Updater) download(ctx context.Context, url string, a Asset, dest string) error {
	resp, err := u.fetch(ctx, url)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	f, err := os.OpenFile(dest, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	h := sha256.New()
	buf := make([]byte, 64<<10)
	var n int64
	last := -1
	for {
		k, rerr := resp.Body.Read(buf)
		if k > 0 {
			n += int64(k)
			if n > a.Size {
				_ = f.Close()
				return errors.New("the file is bigger than the manifest says")
			}
			h.Write(buf[:k])
			if _, werr := f.Write(buf[:k]); werr != nil {
				_ = f.Close()
				return werr
			}
			if pct := int(n * 100 / a.Size); pct != last {
				last = pct
				u.mu.Lock()
				u.status.Pct = pct
				u.mu.Unlock()
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			_ = f.Close()
			return rerr
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	if n != a.Size {
		return errors.New("the file is shorter than the manifest says")
	}
	if !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), a.SHA256) {
		return errors.New("the checksum does not match the signed manifest")
	}
	return nil
}

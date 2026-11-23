// Package remote is the connection of the application to a Linager server: the TLS connection pinned to the
// fingerprint of the server certificate, the session cookie kept on this side (the web view never sees it) and the
// reverse proxy the web view talks to.
package remote

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Fingerprint is "SHA256:<base64>" of the DER bytes of a certificate, the same form the server and the installer show.
func Fingerprint(der []byte) string {
	h := sha256.Sum256(der)
	return "SHA256:" + strings.TrimRight(base64.StdEncoding.EncodeToString(h[:]), "=")
}

// ErrPin means the server presented another certificate than the one that was pinned.
var ErrPin = errors.New("the server certificate does not match the pinned fingerprint")

// Probe connects to host:port and returns the fingerprint of the certificate the server presents. Nothing is sent
// over the connection: this is the "trust on first use" step the person confirms by comparing the fingerprint with
// the one the server shows (installer, Settings, Connection).
func Probe(ctx context.Context, addr string) (string, error) {
	d := tls.Dialer{NetDialer: &net.Dialer{Timeout: 8 * time.Second}, Config: &tls.Config{MinVersion: tls.VersionTLS13, InsecureSkipVerify: true}} //nolint:gosec // only reads the certificate, the person confirms it
	c, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", err
	}
	defer func() { _ = c.Close() }()
	cs := c.(*tls.Conn).ConnectionState()
	if len(cs.PeerCertificates) == 0 {
		return "", errors.New("the server presented no certificate")
	}
	return Fingerprint(cs.PeerCertificates[0].Raw), nil
}

// PinnedTLS is a TLS client configuration that accepts one certificate only, the one with this fingerprint. There is
// no chain to verify: the pin is the trust. The check runs on every connection, resumed ones included.
func PinnedTLS(fp string) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS13,
		InsecureSkipVerify: true, //nolint:gosec // replaced by the pin in VerifyConnection
		VerifyConnection: func(cs tls.ConnectionState) error {
			if len(cs.PeerCertificates) == 0 || Fingerprint(cs.PeerCertificates[0].Raw) != fp {
				return ErrPin
			}
			return nil
		},
	}
}

// Target is a server the application is connected to.
type Target struct {
	Addr        string // host:port of the application port
	Fingerprint string // pinned certificate
	BasePath    string // path prefix of the panel (admin.base_path), usually empty
}

// Session is one connection to a server. It keeps the cookies of the panel, so the sign-in lives here and never
// reaches the web view.
type Session struct {
	T      Target
	jar    *cookiejar.Jar
	tr     *http.Transport
	proxy  *httputil.ReverseProxy
	mu     sync.Mutex
	closed bool
}

// NewSession builds a session for a target.
func NewSession(t Target) (*Session, error) {
	if _, _, err := net.SplitHostPort(t.Addr); err != nil {
		return nil, fmt.Errorf("address must be host:port: %w", err)
	}
	t.BasePath = strings.TrimRight(t.BasePath, "/")
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	s := &Session{T: t, jar: jar}
	s.tr = &http.Transport{
		TLSClientConfig:       PinnedTLS(t.Fingerprint),
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 0, // streams (SSE, terminals) have no header deadline
		IdleConnTimeout:       90 * time.Second,
		ForceAttemptHTTP2:     false, // WebSocket upgrades need HTTP/1.1
	}
	s.proxy = &httputil.ReverseProxy{
		Rewrite:        s.rewrite,
		ModifyResponse: s.modify,
		Transport:      s.tr,
		FlushInterval:  -1, // the live stream and the terminals must not wait for a full buffer
		ErrorHandler: func(w http.ResponseWriter, _ *http.Request, err error) {
			code, msg := http.StatusBadGateway, "unreachable"
			if errors.Is(err, ErrPin) {
				code, msg = http.StatusForbidden, "pin_mismatch"
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(code)
			_, _ = fmt.Fprintf(w, `{"error":%q}`, msg)
		},
	}
	return s, nil
}

func (s *Session) upstream(path string) *url.URL {
	return &url.URL{Scheme: "https", Host: s.T.Addr, Path: s.T.BasePath + path}
}

// rewrite points the request at the server. The cookies come from the jar, never from the web view, and nothing that
// would name the local origin goes upstream (the WebSocket origin check of the server compares Origin with Host).
func (s *Session) rewrite(pr *httputil.ProxyRequest) {
	pr.Out.URL.Scheme = "https"
	pr.Out.URL.Host = s.T.Addr
	pr.Out.URL.Path = s.T.BasePath + pr.In.URL.Path
	if pr.In.URL.RawPath != "" {
		pr.Out.URL.RawPath = s.T.BasePath + pr.In.URL.RawPath
	}
	pr.Out.Host = s.T.Addr
	pr.Out.Header.Del("Cookie")
	for _, c := range s.jar.Cookies(pr.Out.URL) {
		pr.Out.AddCookie(c)
	}
	if pr.Out.Header.Get("Origin") != "" {
		pr.Out.Header.Set("Origin", "https://"+s.T.Addr)
	}
	pr.Out.Header.Del("Referer")
	pr.Out.Header["X-Forwarded-For"] = nil // the proxy does not add the loopback address
}

// modify keeps the cookies the server sets and hides them from the web view.
func (s *Session) modify(resp *http.Response) error {
	if cs := resp.Cookies(); len(cs) > 0 {
		s.jar.SetCookies(resp.Request.URL, cs)
		resp.Header.Del("Set-Cookie")
	}
	return nil
}

// Handler is the reverse proxy; the path of a request is the path on the server.
func (s *Session) Handler() http.Handler { return s.proxy }

// Client is an HTTP client with the pin and the cookies of this session, for calls the application itself makes.
func (s *Session) Client() *http.Client {
	return &http.Client{Transport: s.tr, Jar: s.jar, Timeout: 20 * time.Second}
}

// URL is the address of a path on the server.
func (s *Session) URL(path string) string { return s.upstream(path).String() }

// Close drops idle connections.
func (s *Session) Close() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.closed {
		s.closed = true
		s.tr.CloseIdleConnections()
	}
}

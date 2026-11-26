package local

import (
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// clientHeader is what the application tells the server about itself (X-Linager-Client): the platform, the version of the application
// and of the system, and the name of the computer, which is what a person knows it by in the list of devices.
func clientHeader(version string) string {
	host, _ := os.Hostname()
	q := url.Values{}
	q.Set("platform", platformName())
	q.Set("app", version)
	if v := osVersion(); v != "" {
		q.Set("os", v)
	}
	if host != "" {
		q.Set("model", host)
	}
	return q.Encode()
}

func platformName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macos"
	case "windows", "linux":
		return runtime.GOOS
	}
	return "other"
}

// osVersion is the version of the system as a person writes it ("27.2", "Ubuntu 24.04", "10.0.26100"); empty when it cannot be found.
func osVersion() string {
	switch runtime.GOOS {
	case "darwin":
		return run("sw_vers", "-productVersion")
	case "linux":
		b, err := os.ReadFile("/etc/os-release")
		if err != nil {
			return ""
		}
		return osReleaseName(string(b))
	case "windows":
		// "Microsoft Windows [Version 10.0.26100.1]"
		out := run("cmd", "/c", "ver")
		if i := strings.Index(out, "Version "); i >= 0 {
			return strings.TrimRight(strings.TrimSpace(out[i+len("Version "):]), "]")
		}
	}
	return ""
}

// osReleaseName is PRETTY_NAME of /etc/os-release.
func osReleaseName(s string) string {
	for _, line := range strings.Split(s, "\n") {
		if v, ok := strings.CutPrefix(line, "PRETTY_NAME="); ok {
			return strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return ""
}

func run(name string, args ...string) string {
	done := make(chan string, 1)
	go func() {
		out, err := exec.Command(name, args...).Output() //nolint:gosec // fixed commands that only print the version
		if err != nil {
			done <- ""
			return
		}
		done <- strings.TrimSpace(string(out))
	}()
	select {
	case v := <-done:
		return v
	case <-time.After(2 * time.Second):
		return ""
	}
}

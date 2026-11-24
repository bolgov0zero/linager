//go:build !windows

package agent

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

// AptPackage is one upgradable package.
type AptPackage struct {
	Name      string `json:"name"`
	Current   string `json:"current"`
	Candidate string `json:"candidate"`
	Security  bool   `json:"security"`
	Reboot    bool   `json:"reboot"`
	Ignored   bool   `json:"ignored,omitempty"` // the user does not want to be told about this package
}

var instRe = regexp.MustCompile(`^Inst (\S+)(?: \[([^\]]+)\])? \((\S+) ([^)]*?)(?: \[[^\]]*\])?\)`)

// parseAptInst parses `apt-get -s upgrade` output.
func parseAptInst(out string) []AptPackage {
	var pk []AptPackage
	for _, line := range strings.Split(out, "\n") {
		m := instRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		pk = append(pk, AptPackage{Name: m[1], Current: m[2], Candidate: m[3],
			Security: strings.Contains(strings.ToLower(m[4]), "security"), Reboot: needsReboot(m[1])})
	}
	return pk
}

// needsReboot is a heuristic: packages that usually require a reboot after the upgrade.
func needsReboot(name string) bool {
	return name == "libc6" || name == "systemd" || name == "dbus" || strings.HasPrefix(name, "linux-image") || strings.HasPrefix(name, "linux-base")
}

func aptSimulate(ctx context.Context) ([]AptPackage, error) {
	cmd := exec.CommandContext(ctx, "apt-get", "-s", "-o", "Debug::NoLocking=1", "upgrade", "--with-new-pkgs") //nolint:gosec // fixed command
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		return nil, errors.New("apt-get simulation failed")
	}
	return parseAptInst(string(out)), nil
}

// lastAptCheck returns the unix time of the last successful `apt-get update`, or 0.
func lastAptCheck() int64 {
	var best int64
	for _, p := range []string{"/var/lib/apt/periodic/update-success-stamp", "/var/lib/apt/lists"} {
		if st, err := os.Stat(p); err == nil && st.ModTime().Unix() > best {
			best = st.ModTime().Unix()
		}
	}
	if m, _ := filepath.Glob("/var/lib/apt/lists/*Release"); len(m) > 0 {
		for _, p := range m {
			if st, err := os.Stat(p); err == nil && st.ModTime().Unix() > best {
				best = st.ModTime().Unix()
			}
		}
	}
	return best
}

func aptList(ctx context.Context, _ json.RawMessage, _ bool) (any, error) {
	pk, err := aptSimulate(ctx)
	if err != nil {
		return nil, err
	}
	if pk == nil {
		pk = []AptPackage{}
	}
	for i := range pk {
		pk[i].Ignored = ignores.isApt(pk[i].Name)
	}
	_, rerr := os.Stat("/var/run/reboot-required")
	return map[string]any{"packages": pk, "reboot_required": rerr == nil, "last_check": lastAptCheck(), "busy": aptBusy()}, nil
}

// aptJob is one running apt operation. It is not tied to the stream that started it: closing the
// browser tab or losing the server connection must not kill apt in the middle of an upgrade.
type aptJob struct {
	mu   sync.Mutex
	cond *sync.Cond
	log  []byte // output so far (the tail, at most aptLogMax bytes)
	base int    // number of bytes dropped from the head of log
	done bool

	events  [][]byte // event lines (see aptMark), all of them: there are only a few per package
	prog    aptProg  // latest progress
	progVer int      // changes with prog
}

const aptLogMax = 1 << 20

func (j *aptJob) Write(p []byte) (int, error) {
	j.mu.Lock()
	j.log = append(j.log, p...)
	if over := len(j.log) - aptLogMax; over > 0 {
		j.log = append([]byte(nil), j.log[over:]...)
		j.base += over
	}
	j.cond.Broadcast()
	j.mu.Unlock()
	return len(p), nil
}

// follow copies the job events and output to the stream from the beginning until the job ends or ctx
// is cancelled.
func (j *aptJob) follow(ctx context.Context, sio *StreamIO) error {
	stop := context.AfterFunc(ctx, func() { j.mu.Lock(); j.cond.Broadcast(); j.mu.Unlock() })
	defer stop()
	pos, epos, sentVer := 0, 0, 0
	for {
		j.mu.Lock()
		for pos >= j.base+len(j.log) && epos >= len(j.events) && sentVer == j.progVer && !j.done && ctx.Err() == nil {
			j.cond.Wait()
		}
		pos = max(pos, j.base)
		evs := j.events[epos:] // entries are never modified, only appended to
		prog, ver := j.prog, j.progVer
		chunk := append([]byte(nil), j.log[pos-j.base:]...)
		done := j.done
		j.mu.Unlock()
		sent := false
		for _, e := range evs {
			if err := sio.Out(e); err != nil {
				return err
			}
			sent = true
		}
		epos += len(evs)
		if ver != sentVer {
			if b, err := json.Marshal(prog); err == nil {
				if err := sio.Out(append(append([]byte(aptMark), b...), '\n')); err != nil {
					return err
				}
			}
			sentVer = ver
			sent = true
		}
		if len(chunk) > 0 {
			if err := sio.Out(chunk); err != nil {
				return err
			}
			pos += len(chunk)
			sent = true
		}
		if sent {
			continue
		}
		if done || ctx.Err() != nil {
			return nil
		}
	}
}

var (
	aptMu  sync.Mutex // guards aptCur; one apt operation at a time
	aptCur *aptJob
	// aptCommand builds the command; tests replace it.
	aptCommand = func(args ...string) *exec.Cmd {
		cmd := exec.Command("apt-get", args...) //nolint:gosec // fixed command, package names come from apt itself
		cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive", "LC_ALL=C")
		return cmd
	}
)

// aptBusy reports whether an apt operation started from the panel is still running.
func aptBusy() bool {
	aptMu.Lock()
	defer aptMu.Unlock()
	return aptCur != nil
}

// aptStream runs update / upgrade / security-only upgrade and streams the output. When an operation
// is already running, the stream attaches to it and replays its events and log instead of starting
// another one.
func aptStream(ctx context.Context, raw json.RawMessage, sio *StreamIO, onDone func()) error {
	var p struct {
		Mode  string
		Names []string
	}
	_ = json.Unmarshal(raw, &p)

	aptMu.Lock()
	job := aptCur
	if job == nil {
		// wait up to two minutes for a lock held by another package manager (unattended-upgrades)
		common := []string{"-y", "-q", "-o", "DPkg::Lock::Timeout=120", "-o", "Dpkg::Options::=--force-confdef", "-o", "Dpkg::Options::=--force-confold", "-o", "Dpkg::Progress-Fancy=0"}
		var args []string
		var queue []AptPackage // packages the browser should list as "queued"
		switch p.Mode {
		case "update":
			args = []string{"update", "-q"}
		case "upgrade":
			args = append([]string{"upgrade", "--with-new-pkgs"}, common...)
			queue, _ = aptSimulate(ctx) // without it the list is built from what apt reports
		case "security":
			pk, err := aptSimulate(ctx)
			if err != nil {
				aptMu.Unlock()
				return err
			}
			names := []string{}
			for _, x := range pk {
				if x.Security {
					names = append(names, x.Name)
					queue = append(queue, x)
				}
			}
			if len(names) == 0 {
				aptMu.Unlock()
				return sio.Out([]byte("No security updates available.\n"))
			}
			args = append(append([]string{"install", "--only-upgrade"}, common...), names...)
		case "packages":
			// only the chosen packages, and only those apt itself lists as upgradable
			pk, err := aptSimulate(ctx)
			if err != nil {
				aptMu.Unlock()
				return err
			}
			want := map[string]bool{}
			for _, n := range p.Names {
				want[n] = true
			}
			names := []string{}
			for _, x := range pk {
				if want[x.Name] {
					names = append(names, x.Name)
					queue = append(queue, x)
				}
			}
			if len(names) == 0 {
				aptMu.Unlock()
				return sio.Out([]byte("Nothing to install.\n"))
			}
			args = append(append([]string{"install", "--only-upgrade"}, common...), names...)
		default:
			aptMu.Unlock()
			return errors.New("unknown mode")
		}
		job = &aptJob{}
		job.cond = sync.NewCond(&job.mu)
		tr := newAptTracker(job)
		tr.add(queue)

		// apt reports the state of every package on this descriptor (fd 3 of the child)
		rd, wr, perr := os.Pipe()
		if perr == nil {
			args = append([]string{"-o", "APT::Status-Fd=3"}, args...)
		}
		cmd := aptCommand(args...)
		cmd.Stdout, cmd.Stderr = job, job
		if perr == nil {
			cmd.ExtraFiles = []*os.File{wr}
		}
		if err := cmd.Start(); err != nil {
			if perr == nil {
				_ = rd.Close()
				_ = wr.Close()
			}
			aptMu.Unlock()
			return err
		}
		readDone := make(chan struct{})
		if perr == nil {
			_ = wr.Close() // the child keeps its own copy
			go func() {
				defer close(readDone)
				sc := bufio.NewScanner(rd)
				sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
				for sc.Scan() {
					tr.line(sc.Text())
				}
			}()
		} else {
			close(readDone)
		}
		aptCur = job
		go func() {
			werr := cmd.Wait()
			select {
			case <-readDone:
			case <-time.After(2 * time.Second): // a daemon started by a package may still hold the pipe
				if perr == nil {
					_ = rd.Close()
				}
				<-readDone
			}
			if perr == nil {
				_ = rd.Close()
			}
			tr.finish(werr != nil)
			aptMu.Lock()
			aptCur = nil
			aptMu.Unlock()
			job.mu.Lock()
			job.done = true
			job.cond.Broadcast()
			job.mu.Unlock()
			if onDone != nil {
				onDone()
			}
		}()
	}
	aptMu.Unlock()
	return job.follow(ctx, sio)
}

package config

import (
	"errors"
	"os"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

// AgentModules lists the agent modules of this platform in display order.
var AgentModules = ModulesFor(runtime.GOOS)

// ModulesFor lists the agent modules of a platform ("windows" or anything else for Linux). The Windows agent
// has the modules that map onto Windows directly; the others (apt, firewall, network, users, docker, disk,
// security) are Linux-only.
func ModulesFor(goos string) []string {
	if goos == "windows" {
		return []string{"metrics", "terminal", "desktop", "assistant", "services", "processes", "logs", "files", "disk", "users", "programs", "winupdate", "network"}
	}
	return []string{"metrics", "terminal", "assistant", "services", "apt", "processes", "logs", "files", "disk", "network", "firewall", "security", "users", "docker"}
}

// Agent is /etc/linager/agent.yaml. Every module can be switched off.
type Agent struct {
	Modules map[string]bool `yaml:"modules"`
}

// DefaultAgent enables every module.
func DefaultAgent() *Agent {
	m := map[string]bool{}
	for _, n := range AgentModules {
		m[n] = true
	}
	return &Agent{Modules: m}
}

// LoadAgent reads the agent config; a missing file yields the defaults.
func LoadAgent(path string) (*Agent, error) {
	c := DefaultAgent()
	b, err := os.ReadFile(path) //nolint:gosec // operator-supplied path
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	var in Agent
	if err := yaml.Unmarshal(b, &in); err != nil {
		return nil, err
	}
	for k, v := range in.Modules { // unspecified modules keep their default (enabled)
		c.Modules[k] = v
	}
	return c, nil
}

// Enabled reports whether a module is on.
func (a *Agent) Enabled(name string) bool { return a.Modules[name] }

// List returns the enabled modules in display order.
func (a *Agent) List() []string {
	var out []string
	for _, n := range AgentModules {
		if a.Modules[n] {
			out = append(out, n)
		}
	}
	return out
}

// CompleteAgent adds the modules a newer agent knows to an existing agent.yaml (as enabled, which is what an
// unlisted module is anyway), so the file always lists everything. The rest of the file is left as it is: only
// lines are appended at the end of the block-style `modules:` mapping. A missing file, a flow-style mapping or a
// file without `modules:` stay untouched. It returns the names that were added.
func CompleteAgent(path string) ([]string, error) {
	b, err := os.ReadFile(path) //nolint:gosec // operator-supplied path
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var in Agent
	if err := yaml.Unmarshal(b, &in); err != nil {
		return nil, err
	}
	var missing []string
	for _, n := range AgentModules {
		if _, ok := in.Modules[n]; !ok {
			missing = append(missing, n)
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}
	lines := strings.Split(string(b), "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimRight(l, " \t\r") == "modules:" {
			start = i
			break
		}
	}
	if start < 0 {
		return nil, nil
	}
	last := start // the last indented line of the block
	for i := start + 1; i < len(lines); i++ {
		l := strings.TrimRight(lines[i], " \t\r")
		if l == "" || strings.HasPrefix(strings.TrimLeft(l, " \t"), "#") {
			continue
		}
		if l[0] != ' ' && l[0] != '\t' {
			break
		}
		last = i
	}
	add := make([]string, 0, len(missing))
	for _, n := range missing {
		add = append(add, "  "+n+": true")
	}
	out := append(append(append([]string{}, lines[:last+1]...), add...), lines[last+1:]...)
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	tmp := path + ".linager-new"
	if err := os.WriteFile(tmp, []byte(strings.Join(out, "\n")), st.Mode().Perm()); err != nil { //nolint:gosec // the mode mirrors the existing file
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return nil, err
	}
	return missing, nil
}

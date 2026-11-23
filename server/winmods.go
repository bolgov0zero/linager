package server

import (
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/bolgov0zero/linager/server/store"
)

// The Windows agent has its own users, updates and network modules (their data is not the data of the Linux modules:
// no shells or sudo, but groups and sessions; Windows Update; adapters). One route carries their calls: the module
// and the action are checked against this table, the agent checks the values.
type winAction struct {
	right     string // the operator right the call needs
	dangerous bool   // needs the confirmation, and is written to the audit log
	timeout   time.Duration
}

var winActions = map[string]winAction{
	"users.state":      {"users", false, 30 * time.Second},
	"users.add":        {"users", true, 60 * time.Second},
	"users.update":     {"users", true, 60 * time.Second},
	"users.passwd":     {"users", true, 60 * time.Second},
	"users.delete":     {"users", true, 60 * time.Second},
	"users.logoff":     {"users", true, 30 * time.Second},
	"apps.list":        {"programs", false, 60 * time.Second},
	"winupdate.list":   {"apt", false, 6 * time.Minute},
	"network.state":    {"network", false, 30 * time.Second},
	"network.apply":    {"network", true, 90 * time.Second},
	"network.confirm":  {"network", false, 30 * time.Second},
	"network.rollback": {"network", false, 90 * time.Second},
}

// winSecrets are fields that never reach the audit log.
var winSecrets = map[string]bool{"password": true, "confirm": true}

var winUpdateIDRe = regexp.MustCompile(`^[0-9a-fA-F-]{36}$`)

func (a *Admin) winCall(w http.ResponseWriter, r *http.Request, u *store.User) {
	s := a.serverOr404(w, r)
	if s == nil {
		return
	}
	key := r.PathValue("module") + "." + r.PathValue("action")
	act, ok := winActions[key]
	if !ok {
		jsonErr(w, http.StatusNotFound, "unknown action")
		return
	}
	if !a.moduleOpen(r, u, act.right) {
		jsonErr(w, http.StatusForbidden, "forbidden")
		return
	}
	var params map[string]any
	if !decode(w, r, &params) {
		return
	}
	confirm, _ := params["confirm"].(bool)
	delete(params, "confirm")
	if act.dangerous && !confirm {
		needConfirm(w)
		return
	}
	module, action, _ := strings.Cut(key, ".")
	data, err := a.Hub.Call(r.Context(), s.ID, module, action, params, act.dangerous, act.timeout)
	if act.dangerous || key == "network.rollback" {
		details := map[string]any{}
		for k, v := range params {
			if !winSecrets[k] {
				details[k] = v
			}
		}
		res := "ok"
		if err != nil {
			res = "failed"
		}
		a.audit(r, u, "win."+key, "server", s.ID, s.Name, res, details)
	}
	if err != nil {
		callErr(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data) //nolint:gosec // the JSON answer of our own agent
}

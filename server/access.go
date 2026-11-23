package server

import (
	"net/http"
	"strings"

	"github.com/bolgov0zero/linager/server/store"
)

// Who may call what. Every authenticated route is registered with a rule (see Admin.route), so a new route
// cannot be added without saying who may use it; a test walks the whole table.
//
//	any         any signed-in user (own profile, own dashboard, lists the handler filters itself)
//	admin       administrators only
//	srv         any role, on a server the user may see (the overview and the metrics)
//	srv:<mod>   administrators, and operators who were given the module, on a server they may see
//	srv:admin   administrators only, on a server of the path (delete, reboot, move to a group)
//	mod:<mod>   administrators and operators who were given the module (not tied to a server)
//
// The auditor only reaches "any" and "srv" routes, which are reads or the own data of the user.

// OperatorModules are the agent modules that can be given to an operator, in display order.
var OperatorModules = []string{"terminal", "desktop", "assistant", "files", "services", "apt", "processes", "programs", "logs", "disk", "network", "firewall", "security", "users", "docker"}

// RootModules give the access of root on the host, whatever the panel forbids elsewhere.
var RootModules = []string{"terminal", "desktop", "assistant", "files", "network", "firewall", "security", "users", "docker"}

type routeInfo struct {
	Pattern string
	Spec    string
}

func validModule(m string) bool {
	for _, x := range OperatorModules {
		if x == m {
			return true
		}
	}
	return false
}

// route registers an authenticated route with its access rule.
func (a *Admin) route(mux *http.ServeMux, pattern, spec string, h func(http.ResponseWriter, *http.Request, *store.User)) {
	a.routes = append(a.routes, routeInfo{pattern, spec})
	mux.HandleFunc(pattern, a.auth(a.guard(spec, h)))
}

func (a *Admin) guard(spec string, next func(http.ResponseWriter, *http.Request, *store.User)) func(http.ResponseWriter, *http.Request, *store.User) {
	kind, mod, _ := strings.Cut(spec, ":")
	return func(w http.ResponseWriter, r *http.Request, u *store.User) {
		if u.Role == store.RoleAdmin || kind == "any" {
			next(w, r, u)
			return
		}
		switch kind {
		case "srv":
			if mod == "admin" {
				jsonErr(w, http.StatusForbidden, "forbidden")
				return
			}
			if !a.serverVisible(r, u) {
				jsonErr(w, http.StatusNotFound, "server not found") // a server the user may not see does not exist for them
				return
			}
			if mod != "" && !a.moduleOpen(r, u, mod) {
				jsonErr(w, http.StatusForbidden, "forbidden")
				return
			}
			next(w, r, u)
		case "mod":
			if !a.moduleOpen(r, u, mod) {
				jsonErr(w, http.StatusForbidden, "forbidden")
				return
			}
			next(w, r, u)
		default: // "admin"
			jsonErr(w, http.StatusForbidden, "forbidden")
		}
	}
}

// serverVisible reports whether the user may see the server of the path.
func (a *Admin) serverVisible(r *http.Request, u *store.User) bool {
	if u.Role == store.RoleAdmin {
		return true
	}
	ok, err := a.DB.ServerAllowed(r.Context(), u.ID, r.PathValue("id"))
	return err == nil && ok
}

// moduleOpen reports whether the user may use an agent module: administrators always, operators when they
// were given it, auditors never.
func (a *Admin) moduleOpen(r *http.Request, u *store.User, module string) bool {
	switch u.Role {
	case store.RoleAdmin:
		return true
	case store.RoleOperator:
		ok, err := a.DB.ModuleAllowed(r.Context(), u.ID, module)
		return err == nil && ok
	}
	return false
}

// streamModule is the agent module a stream of the given kind belongs to.
func streamModule(kind string) string {
	switch {
	case kind == "terminal", kind == "logs", kind == "apt":
		return kind
	case kind == "winupdate":
		return "apt" // Windows Update is the updates right of the operator
	case strings.HasPrefix(kind, "docker-"):
		return "docker"
	case kind == "f2b-install":
		return "security"
	}
	return ""
}

// visibleServers returns the ids of the servers the user may see (nil for an administrator: all of them).
func (a *Admin) visibleServers(r *http.Request, u *store.User) (map[string]bool, error) {
	if u.Role == store.RoleAdmin {
		return nil, nil
	}
	return a.DB.AllowedServerIDs(r.Context(), u.ID)
}

//go:build windows

package agent

import (
	"context"
	"log/slog"

	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/internal/version"
)

// registerPlatform registers the modules of a Windows host: terminal, services, processes, event log and files.
// The other modules of the Linux agent (apt, firewall, network, users, docker, disk, security) have no Windows
// counterpart yet.
func (r *Registry) registerPlatform(cfg *config.Agent, _ Collector, _ *slog.Logger, dataDir string) {
	r.cmds["update.apply"] = r.updateApplyCmd(version.Version) // the panel updates the agent: the signed exe replaces the running one
	if cfg.Enabled("terminal") {
		r.streams["terminal"] = termStream
	}
	if cfg.Enabled("services") {
		r.cmds["services.list"] = svcList
		r.cmds["services.action"] = svcAction
	}
	if cfg.Enabled("processes") {
		r.cmds["processes.list"] = procList
		r.cmds["processes.kill"] = procKill
	}
	if cfg.Enabled("logs") {
		r.streams["logs"] = logsStream
	}
	if cfg.Enabled("desktop") {
		r.streams["desktop"] = desktopStream
		r.cmds["desktop.state"] = desktopStateCmd
		r.cmds["desktop.enable"] = desktopEnableCmd
		r.cmds["desktop.disable"] = desktopDisableCmd
	}
	if cfg.Enabled("assistant") {
		r.streams["assistant"] = assistantStream
		r.cmds["assistant.sessions"] = assistSessionsCmd
		r.cmds["assistant.sas"] = assistSasCmd
	}
	if cfg.Enabled("users") {
		r.cmds["users.state"] = usersStateCmd
		r.cmds["users.add"] = userAddCmd
		r.cmds["users.update"] = userUpdateCmd
		r.cmds["users.passwd"] = userPasswdCmd
		r.cmds["users.delete"] = userDeleteCmd
		r.cmds["users.logoff"] = userLogoffCmd
	}
	if cfg.Enabled("programs") {
		r.cmds["apps.list"] = appsListCmd
	}
	if cfg.Enabled("winupdate") {
		r.cmds["winupdate.list"] = winUpdateListCmd
		r.streams["winupdate"] = winUpdateStream
	}
	if cfg.Enabled("network") {
		r.cmds["network.state"] = networkStateCmd
		r.cmds["network.apply"] = networkApplyCmd
		r.cmds["network.confirm"] = networkConfirmCmd
		r.cmds["network.rollback"] = networkRollbackCmd
		netDataDir = func() string { return dataDir }
		go recoverNetChange() // a change left unconfirmed by a restart is undone
	}
	if cfg.Enabled("disk") {
		dm := newDiskModule(dataDir)
		dm.start()
		r.cmds["disk.state"] = dm.stateCmd
		r.cmds["disk.scan"] = dm.scanCmd
		r.cmds["disk.clean"] = dm.cleanCmd
	}
	if cfg.Enabled("files") {
		r.cmds["files.list"] = filesList
		r.cmds["files.read"] = filesRead
		r.cmds["files.write"] = filesWrite
		r.cmds["files.mkdir"] = filesMkdir
		r.cmds["files.create"] = filesCreate
		r.cmds["files.delete"] = filesDelete
		r.cmds["files.rename"] = filesRename
		r.cmds["files.stat"] = filesStat
		r.cmds["files.du"] = filesDu
		r.cmds["files.copy"] = transfer(false)
		r.cmds["files.move"] = transfer(true)
		r.streams["files.zip"] = filesZip
		r.streams["files.download"] = filesDownload
		r.streams["files.upload"] = filesUpload
	}
	r.cmds["system.reboot"] = sysReboot // always available: rebooting is not a toggleable module
}

// startBackground starts the work that runs next to the connection: nothing on Windows.
func (r *Registry) startBackground(context.Context) {}

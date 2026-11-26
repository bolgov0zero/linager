package server

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/bolgov0zero/linager/server/store"
)

// The commands `linager server devices ...`: the way back in when the only approved device is lost. Whoever can run them has the
// server itself, which is more than any application. They write the audit like the application does (the actor is "cli").

// DevicesList prints the devices and whether the confirmation is on.
func DevicesList(ctx context.Context, db *store.Store, w io.Writer) error {
	if since := parseGate(db.Setting(ctx, store.DeviceApprovalSetting)); !since.IsZero() {
		_, _ = fmt.Fprintf(w, "Confirmation of devices: ON (since %s)\n", since.Local().Format("2006-01-02 15:04"))
	} else {
		_, _ = fmt.Fprintln(w, "Confirmation of devices: off")
	}
	list, err := db.ListDevices(ctx)
	if err != nil {
		return err
	}
	if len(list) == 0 {
		_, err = fmt.Fprintln(w, "No devices yet.")
		return err
	}
	_, _ = fmt.Fprintf(w, "%-5s %-9s %-9s %-14s %-10s %-16s %s\n", "ID", "STATUS", "USER", "DEVICE", "APP", "LAST IP", "LAST SEEN")
	for _, d := range list {
		what := strings.TrimSpace(d.Platform + " " + d.Model)
		if d.Label != "" {
			what = d.Label
		}
		_, _ = fmt.Fprintf(w, "%-5d %-9s %-9s %-14s %-10s %-16s %s\n", d.ID, d.Status, d.Login, clip(what, 14), clip(d.AppVersion, 10), d.LastIP, d.LastSeen.Local().Format("2006-01-02 15:04"))
	}
	return nil
}

func clip(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

// DeviceApproveCLI approves a device by its id.
func DeviceApproveCLI(ctx context.Context, db *store.Store, id int64) error {
	d, err := db.DeviceByID(ctx, id)
	if err != nil {
		return fmt.Errorf("no device with id %d", id)
	}
	if err := db.SetDeviceStatus(ctx, id, store.DeviceApproved, 0); err != nil {
		return err
	}
	return db.WriteAudit(ctx, store.Audit{Actor: "cli", Action: "device.approve", TargetType: "device", TargetID: fmt.Sprint(id), TargetName: deviceName(d),
		Details: map[string]any{"user": d.Login, "platform": d.Platform, "model": d.Model, "via": "cli"}})
}

// DevicesGateOff turns the confirmation of devices off (the running server notices it within seconds).
func DevicesGateOff(ctx context.Context, db *store.Store) error {
	if db.Setting(ctx, store.DeviceApprovalSetting) == "" {
		return nil
	}
	if err := db.SetSetting(ctx, store.DeviceApprovalSetting, ""); err != nil {
		return err
	}
	return db.WriteAudit(ctx, store.Audit{Actor: "cli", Action: "device.gate.off", TargetType: "settings", TargetName: "devices", Details: map[string]any{"via": "cli", "at": time.Now().UTC().Format(time.RFC3339)}})
}

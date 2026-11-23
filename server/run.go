package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bolgov0zero/linager/internal/config"
	"github.com/bolgov0zero/linager/server/store"
)

// CreateAdmin creates the first (and on this version only) administrator.
func CreateAdmin(ctx context.Context, db *store.Store, login, password string) error {
	login = strings.ToLower(strings.TrimSpace(login))
	if login == "" {
		return errors.New("login is required")
	}
	if n, err := db.UserCount(ctx); err != nil {
		return err
	} else if n > 0 {
		return errors.New("an administrator already exists")
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	_, err = db.CreateAdmin(ctx, login, hash)
	return err
}

// ResetPassword sets a new password for an existing user, clears the lock-out and signs the user out everywhere.
// It is the way back when the administrator has lost the password: `linager server reset-password` on the server.
func ResetPassword(ctx context.Context, db *store.Store, login, password string) error {
	login = strings.ToLower(strings.TrimSpace(login))
	u, err := db.UserByLogin(ctx, login)
	if err != nil {
		return fmt.Errorf("user %q: %w", login, err)
	}
	hash, err := HashPassword(password)
	if err != nil {
		return err
	}
	if err := db.SetPassword(ctx, u.ID, hash); err != nil {
		return err
	}
	if err := db.RecordLoginSuccess(ctx, u.ID); err != nil {
		return err
	}
	if _, err := db.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1`, u.ID); err != nil {
		return err
	}
	_ = db.WriteAudit(ctx, store.Audit{Actor: "cli", Action: "user.password_reset", TargetType: "user", TargetName: login, Result: "ok"})
	return nil
}

// Run starts the admin and agent listeners and blocks until ctx is cancelled.
func Run(ctx context.Context, cfg *config.Server, static fs.FS, log *slog.Logger) error {
	db, err := store.Open(ctx, cfg.Database.DSN)
	if err != nil {
		return err
	}
	defer db.Close()
	keys, err := LoadKeys(cfg.DataDir)
	if err != nil {
		return err
	}
	hub := NewHub()
	live := NewLive()
	metrics := NewMetrics(db, live, log)
	updates, err := NewUpdates(db, hub, cfg, log)
	if err != nil {
		return err
	}
	hub.OnChange = func(id string, online bool) {
		go metrics.Status(ctx, id, online)
		go updates.AgentOnline(ctx, id, online)
	}
	go updates.Run(ctx)
	admin, err := NewAdmin(db, keys, hub, live, metrics, updates, cfg, static, log)
	if err != nil {
		return err
	}
	go admin.SSH.Run(ctx)
	go admin.RunJoinPurge(ctx)
	agents := NewAgentPort(db, keys, hub, metrics, updates, log)
	agents.CertLifetime = cfg.AgentCertLifetime()
	agents.Live = live
	conn, err := NewConnection(ctx, cfg, db, hub, agents, keys, log)
	if err != nil {
		return err
	}
	agents.Conn = conn
	admin.Conn = conn
	conn.ssh = admin.SSH
	admin.SSH.SetPort(strconv.Itoa(conn.State().AgentPort)) // the port the panel stored wins over server.yaml
	go (&Scheduler{DB: db, Log: log}).Run(ctx)
	go admin.Access.Run(ctx)

	adminHandler := admin.Handler()
	adminSrv := &http.Server{Addr: cfg.Admin.Listen, Handler: adminHandler, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 64 << 10}
	adminLn, err := net.Listen("tcp", cfg.Admin.Listen)
	if err != nil {
		return err
	}
	errc := make(chan error, 4)
	if err := conn.Start(errc, adminHandler); err != nil {
		_ = adminLn.Close()
		return err
	}
	go func() { errc <- adminSrv.Serve(adminLn) }()
	go func() {
		t := time.NewTicker(10 * time.Minute)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := db.PurgeExpired(ctx); err != nil {
					log.Warn("purge expired", "err", err)
				}
			}
		}
	}()
	st := conn.State()
	log.Info("linager server started", "admin", cfg.Admin.Listen, "agents", cfg.Agents.Listen, "agent_port", st.AgentPort, "app_port", st.AppPort, "move_from", st.LegacyPort)

	select {
	case <-ctx.Done():
	case err = <-errc:
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = adminSrv.Shutdown(shutdown)
	conn.Shutdown(shutdown)
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	return err
}

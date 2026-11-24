package store

import (
	"context"
	"time"
)

// AccessLogRow is one connection to the application port as the application shows it.
type AccessLogRow struct {
	ID         int64  `json:"id"`
	TS         int64  `json:"ts"`
	ClientIP   string `json:"client_ip"`
	RemoteAddr string `json:"remote_addr"`
	Event      string `json:"event"`
	Login      string `json:"login"`
	Path       string `json:"path"`
	UserAgent  string `json:"user_agent"`
	Reason     string `json:"reason"`
}

// Access log events.
const (
	AccessPage      = "page"
	AccessLoginOK   = "login.ok"
	AccessLoginFail = "login.fail"
	AccessDenied    = "denied"
)

// maxAccessLogRows is the hard cap of the journal, whatever the retention is (a scanner must not fill the disk).
const maxAccessLogRows = 50000

// InsertAccessLog appends a visit.
func (s *Store) InsertAccessLog(ctx context.Context, r AccessLogRow) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO access_log (client_ip, remote_addr, event, login, path, user_agent, reason)
		VALUES ($1,$2,$3,$4,$5,$6,$7)`, r.ClientIP, r.RemoteAddr, r.Event, r.Login, clip(r.Path, 200), clip(r.UserAgent, 200), r.Reason)
	return err
}

// clip shortens a text taken from a request (it is attacker controlled) to n bytes at a rune boundary.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && s[n]&0xC0 == 0x80 {
		n--
	}
	return s[:n]
}

// AccessLogList returns visits newest first. filter is one of all, denied, login, fail.
func (s *Store) AccessLogList(ctx context.Context, filter string, before int64, limit int) ([]AccessLogRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := s.Pool.Query(ctx, `SELECT id, extract(epoch FROM ts)::bigint, client_ip, remote_addr, event, login, path, user_agent, reason
		FROM access_log WHERE ($1::bigint = 0 OR id < $1) AND (
			$2 = 'all' OR ($2 = 'denied' AND event = 'denied') OR ($2 = 'login' AND event LIKE 'login.%')
			OR ($2 = 'fail' AND event = 'login.fail'))
		ORDER BY id DESC LIMIT $3`, before, filter, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AccessLogRow{}
	for rows.Next() {
		var r AccessLogRow
		if err := rows.Scan(&r.ID, &r.TS, &r.ClientIP, &r.RemoteAddr, &r.Event, &r.Login, &r.Path, &r.UserAgent, &r.Reason); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AccessDeniedSince counts the refused requests since the given time.
func (s *Store) AccessDeniedSince(ctx context.Context, since time.Time) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM access_log WHERE event = 'denied' AND ts >= $1`, since).Scan(&n)
	return n, err
}

// PurgeAccessLog removes the visits older than the retention and everything above the row cap.
func (s *Store) PurgeAccessLog(ctx context.Context, retention time.Duration) error {
	if _, err := s.Pool.Exec(ctx, `DELETE FROM access_log WHERE ts < $1`, time.Now().Add(-retention)); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `DELETE FROM access_log WHERE id <= (SELECT id FROM access_log ORDER BY id DESC OFFSET $1 LIMIT 1)`, maxAccessLogRows)
	return err
}

// SetSettingRaw stores a setting given as JSON text (the "access" setting is a JSON object).
func (s *Store) SetSettingRaw(ctx context.Context, key string, raw []byte) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO settings (key, value) VALUES ($1,$2) ON CONFLICT (key) DO UPDATE SET value=EXCLUDED.value`, key, raw)
	return err
}

// SettingRaw returns the JSON value of a setting (nil when missing).
func (s *Store) SettingRaw(ctx context.Context, key string) []byte {
	var raw []byte
	if err := s.Pool.QueryRow(ctx, `SELECT value FROM settings WHERE key=$1`, key).Scan(&raw); err != nil {
		return nil
	}
	return raw
}

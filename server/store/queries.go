package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// User is an administrator account.
type User struct {
	ID             int64
	Login          string
	PasswordHash   string
	TOTPSecret     string
	TOTPEnabled    bool
	TOTPLastStep   int64
	FailedAttempts int
	LockedUntil    *time.Time
	// Role is filled by the session check (admin, operator or auditor); other queries leave it empty.
	Role string
}

const userCols = `id, login, password_hash, coalesce(totp_secret,''), totp_enabled, totp_last_step, failed_attempts, locked_until`

func scanUser(r pgx.Row) (*User, error) {
	var u User
	err := r.Scan(&u.ID, &u.Login, &u.PasswordHash, &u.TOTPSecret, &u.TOTPEnabled, &u.TOTPLastStep, &u.FailedAttempts, &u.LockedUntil)
	if err != nil {
		return nil, notFound(err)
	}
	return &u, nil
}

// CreateAdmin inserts a user with the admin role and a default dashboard.
func (s *Store) CreateAdmin(ctx context.Context, login, hash string) (*User, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	u, err := scanUser(tx.QueryRow(ctx, `INSERT INTO users (login, password_hash) VALUES ($1,$2) RETURNING `+userCols, login, hash))
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO user_roles SELECT $1, id FROM roles WHERE name='admin'`, u.ID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO dashboards (user_id) VALUES ($1)`, u.ID); err != nil {
		return nil, err
	}
	return u, tx.Commit(ctx)
}

// UserByLogin loads a user by login.
func (s *Store) UserByLogin(ctx context.Context, login string) (*User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE login=$1`, login))
}

// UserByID loads a user by id.
func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.Pool.QueryRow(ctx, `SELECT `+userCols+` FROM users WHERE id=$1`, id))
}

// UserCount returns the number of users.
func (s *Store) UserCount(ctx context.Context) (n int, err error) {
	err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return
}

// SetPassword replaces a user's password hash.
func (s *Store) SetPassword(ctx context.Context, id int64, hash string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET password_hash=$2 WHERE id=$1`, id, hash)
	return err
}

// RecordLoginFailure increments the failure counter and sets a lock-out when needed.
func (s *Store) RecordLoginFailure(ctx context.Context, id int64, lockFor time.Duration) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET failed_attempts=failed_attempts+1,
		locked_until = CASE WHEN $2 > 0 THEN now() + make_interval(secs => $2) ELSE locked_until END WHERE id=$1`, id, lockFor.Seconds())
	return err
}

// RecordLoginSuccess clears the failure counter.
func (s *Store) RecordLoginSuccess(ctx context.Context, id int64) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET failed_attempts=0, locked_until=NULL WHERE id=$1`, id)
	return err
}

// SetTOTP stores the TOTP secret and enabled flag.
func (s *Store) SetTOTP(ctx context.Context, id int64, secret string, enabled bool) error {
	_, err := s.Pool.Exec(ctx, `UPDATE users SET totp_secret=NULLIF($2,''), totp_enabled=$3, totp_last_step=0 WHERE id=$1`, id, secret, enabled)
	return err
}

// AdvanceTOTPStep records a used time step; it returns false if the step was already used.
func (s *Store) AdvanceTOTPStep(ctx context.Context, id, step int64) (bool, error) {
	ct, err := s.Pool.Exec(ctx, `UPDATE users SET totp_last_step=$2 WHERE id=$1 AND totp_last_step<$2`, id, step)
	return ct.RowsAffected() == 1, err
}

// Session is a logged-in browser session.
type Session struct {
	ID      []byte
	UserID  int64
	CSRF    string
	Expires time.Time
}

// CreateSession stores a session keyed by the hash of its token.
// A sliding session is renewed by every request: it ends after ttl without one.
func (s *Store) CreateSession(ctx context.Context, idHash []byte, userID int64, csrf, ip, ua string, ttl time.Duration, sliding bool) error {
	secs := 0
	if sliding {
		secs = int(ttl.Seconds())
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO sessions (id,user_id,csrf,ip,user_agent,expires_at,sliding_secs) VALUES ($1,$2,$3,$4,$5,now()+make_interval(secs => $6),$7)`,
		idHash, userID, csrf, ip, ua, ttl.Seconds(), secs)
	return err
}

// SessionByHash returns a non-expired session and extends its last-seen time.
func (s *Store) SessionByHash(ctx context.Context, idHash []byte) (*Session, error) {
	var se Session
	err := s.Pool.QueryRow(ctx, `UPDATE sessions SET last_seen=now(), expires_at=CASE WHEN sliding_secs>0 THEN now()+make_interval(secs => sliding_secs) ELSE expires_at END WHERE id=$1 AND expires_at>now() RETURNING id,user_id,csrf,expires_at`, idHash).
		Scan(&se.ID, &se.UserID, &se.CSRF, &se.Expires)
	if err != nil {
		return nil, notFound(err)
	}
	return &se, nil
}

// DeleteSession removes a session.
func (s *Store) DeleteSession(ctx context.Context, idHash []byte) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1`, idHash)
	return err
}

// PurgeExpired deletes expired sessions and finished enrollments.
func (s *Store) PurgeExpired(ctx context.Context) error {
	if _, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE expires_at<now()`); err != nil {
		return err
	}
	_, err := s.Pool.Exec(ctx, `UPDATE enrollments SET status='expired' WHERE status='pending' AND expires_at<now()`)
	if err != nil {
		return err
	}
	_, err = s.Pool.Exec(ctx, `DELETE FROM enrollments WHERE status<>'pending' AND created_at<now()-interval '1 day'`)
	return err
}

// Audit is one audit log entry.
type Audit struct {
	UserID     *int64
	Actor      string
	Action     string
	TargetType string
	TargetID   string
	TargetName string
	RemoteAddr string
	ClientIP   string
	Result     string
	Details    map[string]any
}

// WriteAudit appends an audit record.
func (s *Store) WriteAudit(ctx context.Context, a Audit) error {
	if a.Result == "" {
		a.Result = "ok"
	}
	d, _ := json.Marshal(a.Details)
	if a.Details == nil {
		d = []byte("{}")
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO audit_log (user_id,actor,action,target_type,target_id,target_name,remote_addr,client_ip,result,details)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, a.UserID, a.Actor, a.Action, a.TargetType, a.TargetID, a.TargetName, a.RemoteAddr, a.ClientIP, a.Result, d)
	return err
}

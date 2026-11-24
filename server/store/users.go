package store

import (
	"context"
	"time"
)

// UserRow is a user as shown in the users list.
type UserRow struct {
	ID           int64
	Login        string
	TOTPEnabled  bool
	RecoveryLeft int
	CreatedAt    time.Time
	Role         string
}

// ListUsers returns all users with the state of their second factors.
func (s *Store) ListUsers(ctx context.Context) ([]UserRow, error) {
	rows, err := s.Pool.Query(ctx, `SELECT u.id, u.login, u.totp_enabled, u.created_at,
		(SELECT count(*) FROM recovery_codes c WHERE c.user_id=u.id AND c.used_at IS NULL),
		coalesce((SELECT r.name FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=u.id ORDER BY r.id LIMIT 1), 'admin')
		FROM users u ORDER BY u.login`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []UserRow{}
	for rows.Next() {
		var r UserRow
		if err := rows.Scan(&r.ID, &r.Login, &r.TOTPEnabled, &r.CreatedAt, &r.RecoveryLeft, &r.Role); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DeleteUser removes a user; sessions, codes and the dashboard go with it.
func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	ct, err := s.Pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSessionsOf ends all sessions of a user except the one with the given hash (nil: all of them).
func (s *Store) DeleteSessionsOf(ctx context.Context, userID int64, except []byte) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1 AND ($2::bytea IS NULL OR id<>$2)`, userID, except)
	return err
}

// ---- recovery codes ----

// ReplaceRecoveryCodes drops all codes of the user and stores the new hashes.
func (s *Store) ReplaceRecoveryCodes(ctx context.Context, userID int64, hashes [][]byte) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `DELETE FROM recovery_codes WHERE user_id=$1`, userID); err != nil {
		return err
	}
	for _, h := range hashes {
		if _, err := tx.Exec(ctx, `INSERT INTO recovery_codes (user_id, code_hash) VALUES ($1,$2)`, userID, h); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// RecoveryRemaining counts the unused recovery codes of a user.
func (s *Store) RecoveryRemaining(ctx context.Context, userID int64) (n int, err error) {
	err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM recovery_codes WHERE user_id=$1 AND used_at IS NULL`, userID).Scan(&n)
	return
}

// UseRecoveryCode burns an unused code with this hash; it reports whether one was found.
func (s *Store) UseRecoveryCode(ctx context.Context, userID int64, hash []byte) (bool, error) {
	ct, err := s.Pool.Exec(ctx, `UPDATE recovery_codes SET used_at=now() WHERE id =
		(SELECT id FROM recovery_codes WHERE user_id=$1 AND code_hash=$2 AND used_at IS NULL LIMIT 1)`, userID, hash)
	return ct.RowsAffected() == 1, err
}

// ClearSecondFactors removes the TOTP secret and the recovery codes of a user.
func (s *Store) ClearSecondFactors(ctx context.Context, userID int64) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for _, q := range []string{
		`UPDATE users SET totp_secret=NULL, totp_enabled=false, totp_last_step=0 WHERE id=$1`,
		`DELETE FROM recovery_codes WHERE user_id=$1`,
	} {
		if _, err := tx.Exec(ctx, q, userID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// ---- snippets ----

// Snippet is a saved terminal command.
type Snippet struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Command  string `json:"command"`
	Position int    `json:"position"`
}

// Snippets lists all snippets in their order.
func (s *Store) Snippets(ctx context.Context) ([]Snippet, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, name, command, position FROM snippets ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Snippet{}
	for rows.Next() {
		var sn Snippet
		if err := rows.Scan(&sn.ID, &sn.Name, &sn.Command, &sn.Position); err != nil {
			return nil, err
		}
		out = append(out, sn)
	}
	return out, rows.Err()
}

// SnippetByID loads one snippet.
func (s *Store) SnippetByID(ctx context.Context, id int64) (*Snippet, error) {
	var sn Snippet
	err := s.Pool.QueryRow(ctx, `SELECT id, name, command, position FROM snippets WHERE id=$1`, id).Scan(&sn.ID, &sn.Name, &sn.Command, &sn.Position)
	if err != nil {
		return nil, notFound(err)
	}
	return &sn, nil
}

// CreateSnippet appends a snippet to the end of the list.
func (s *Store) CreateSnippet(ctx context.Context, name, command string, by int64) (*Snippet, error) {
	var sn Snippet
	err := s.Pool.QueryRow(ctx, `INSERT INTO snippets (name, command, position, created_by)
		VALUES ($1,$2,(SELECT COALESCE(max(position)+1,0) FROM snippets),$3) RETURNING id, name, command, position`, name, command, by).
		Scan(&sn.ID, &sn.Name, &sn.Command, &sn.Position)
	return &sn, err
}

// UpdateSnippet changes name and command.
func (s *Store) UpdateSnippet(ctx context.Context, id int64, name, command string) error {
	ct, err := s.Pool.Exec(ctx, `UPDATE snippets SET name=$2, command=$3 WHERE id=$1`, id, name, command)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteSnippet removes a snippet.
func (s *Store) DeleteSnippet(ctx context.Context, id int64) error {
	ct, err := s.Pool.Exec(ctx, `DELETE FROM snippets WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderSnippets sets the order to the given list of ids (ids that are not listed keep their place after them).
func (s *Store) ReorderSnippets(ctx context.Context, ids []int64) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	for i, id := range ids {
		if _, err := tx.Exec(ctx, `UPDATE snippets SET position=$2 WHERE id=$1`, id, i); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE snippets SET position=position+$1 WHERE id<>ALL($2::bigint[]) AND position<$1`, len(ids), ids); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

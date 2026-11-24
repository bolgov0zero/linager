package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// dbq is what a query needs: the pool or a transaction (the previews of a change read inside the transaction they roll back).
type dbq interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// Roles of a user.
const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleAuditor  = "auditor"
)

// Access is what a non-administrator may reach: single servers, whole groups, every server with a tag, and modules.
// The servers, the groups and the tags add up: a server is visible when any of them covers it.
type Access struct {
	Servers []string `json:"servers"`
	Groups  []int64  `json:"groups"`
	Tags    []int64  `json:"tags"`
	Modules []string `json:"modules"`
}

// RoleOf returns the role of a user ("admin" when none is recorded: the accounts of earlier versions).
func (s *Store) RoleOf(ctx context.Context, userID int64) (string, error) {
	var role string
	err := s.Pool.QueryRow(ctx, `SELECT r.name FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=$1 ORDER BY r.id LIMIT 1`, userID).Scan(&role)
	if err != nil {
		if errors.Is(notFound(err), ErrNotFound) {
			return RoleAdmin, nil
		}
		return "", err
	}
	return role, nil
}

// UserAccess returns the servers, groups, tags and modules given to a user.
func (s *Store) UserAccess(ctx context.Context, userID int64) (Access, error) {
	a := Access{Servers: []string{}, Groups: []int64{}, Tags: []int64{}, Modules: []string{}}
	rows, err := s.Pool.Query(ctx, `SELECT server_id::text FROM user_servers WHERE user_id=$1 ORDER BY 1`, userID)
	if err != nil {
		return a, err
	}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return a, err
		}
		a.Servers = append(a.Servers, id)
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `SELECT group_id FROM user_groups WHERE user_id=$1 ORDER BY 1`, userID)
	if err != nil {
		return a, err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return a, err
		}
		a.Groups = append(a.Groups, id)
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `SELECT tag_id FROM user_tags WHERE user_id=$1 ORDER BY 1`, userID)
	if err != nil {
		return a, err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return a, err
		}
		a.Tags = append(a.Tags, id)
	}
	rows.Close()
	rows, err = s.Pool.Query(ctx, `SELECT module FROM user_modules WHERE user_id=$1 ORDER BY 1`, userID)
	if err != nil {
		return a, err
	}
	defer rows.Close()
	for rows.Next() {
		var m string
		if err := rows.Scan(&m); err != nil {
			return a, err
		}
		a.Modules = append(a.Modules, m)
	}
	return a, rows.Err()
}

// SetUserAccess replaces the role and the access of a user in one transaction. Servers, groups and tags that do
// not exist are an error, so a typo or a stale id never silently gives access to nothing.
func (s *Store) SetUserAccess(ctx context.Context, userID int64, role string, a Access) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	ct, err := tx.Exec(ctx, `UPDATE user_roles SET role_id=(SELECT id FROM roles WHERE name=$2) WHERE user_id=$1`, userID, role)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		if _, err := tx.Exec(ctx, `INSERT INTO user_roles SELECT $1, id FROM roles WHERE name=$2`, userID, role); err != nil {
			return err
		}
	}
	for _, q := range []string{`DELETE FROM user_servers WHERE user_id=$1`, `DELETE FROM user_groups WHERE user_id=$1`, `DELETE FROM user_tags WHERE user_id=$1`, `DELETE FROM user_modules WHERE user_id=$1`} {
		if _, err := tx.Exec(ctx, q, userID); err != nil {
			return err
		}
	}
	for _, id := range a.Servers {
		ct, err := tx.Exec(ctx, `INSERT INTO user_servers (user_id, server_id) SELECT $1, id FROM servers WHERE id::text=$2 ON CONFLICT DO NOTHING`, userID, id)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return errors.New("unknown server " + id)
		}
	}
	for _, id := range a.Groups {
		ct, err := tx.Exec(ctx, `INSERT INTO user_groups (user_id, group_id) SELECT $1, id FROM server_groups WHERE id=$2 ON CONFLICT DO NOTHING`, userID, id)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			return errors.New("unknown group")
		}
	}
	for _, id := range a.Tags {
		ct, err := tx.Exec(ctx, `INSERT INTO user_tags (user_id, tag_id) SELECT $1, id FROM tags WHERE id=$2 ON CONFLICT DO NOTHING`, userID, id)
		if err != nil {
			return err
		}
		if ct.RowsAffected() == 0 {
			var has bool // a repeated id is not an error, an unknown one is
			if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_tags WHERE user_id=$1 AND tag_id=$2)`, userID, id).Scan(&has); err != nil || !has {
				return errors.New("unknown tag")
			}
		}
	}
	for _, m := range a.Modules {
		if _, err := tx.Exec(ctx, `INSERT INTO user_modules (user_id, module) VALUES ($1,$2) ON CONFLICT DO NOTHING`, userID, m); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// grantedFolders is the recursive part of the access queries: the folders given to a user and every folder under them.
const grantedFolders = `WITH RECURSIVE gf(id) AS (
		SELECT group_id FROM user_groups WHERE user_id=$1
		UNION
		SELECT c.id FROM server_groups c JOIN gf ON c.parent_id=gf.id)`

// AllowedServerIDs returns the ids of the servers a user may see: the single ones, those in the folders given (with the folders
// under them) and those with a tag given.
func (s *Store) AllowedServerIDs(ctx context.Context, userID int64) (map[string]bool, error) {
	return allowedServerIDs(ctx, s.Pool, userID)
}

func allowedServerIDs(ctx context.Context, q dbq, userID int64) (map[string]bool, error) {
	rows, err := q.Query(ctx, grantedFolders+`
		SELECT server_id::text FROM user_servers WHERE user_id=$1
		UNION SELECT s.id::text FROM servers s WHERE s.group_id IN (SELECT id FROM gf)
		UNION SELECT st.server_id::text FROM server_tags st JOIN user_tags ut ON ut.tag_id=st.tag_id WHERE ut.user_id=$1`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// ServerAllowed reports whether a user may see one server.
func (s *Store) ServerAllowed(ctx context.Context, userID int64, serverID string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, grantedFolders+`
		SELECT EXISTS (SELECT 1 FROM user_servers WHERE user_id=$1 AND server_id::text=$2)
		OR EXISTS (SELECT 1 FROM servers s WHERE s.id::text=$2 AND s.group_id IN (SELECT id FROM gf))
		OR EXISTS (SELECT 1 FROM server_tags st JOIN user_tags ut ON ut.tag_id=st.tag_id WHERE ut.user_id=$1 AND st.server_id::text=$2)`, userID, serverID).Scan(&ok)
	return ok, err
}

// ModuleAllowed reports whether an operator was given a module.
func (s *Store) ModuleAllowed(ctx context.Context, userID int64, module string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM user_modules WHERE user_id=$1 AND module=$2)`, userID, module).Scan(&ok)
	return ok, err
}

// AdminCount returns the number of administrators.
func (s *Store) AdminCount(ctx context.Context) (n int, err error) {
	err = s.Pool.QueryRow(ctx, `SELECT count(*) FROM users u WHERE NOT EXISTS (SELECT 1 FROM user_roles ur JOIN roles r ON r.id=ur.role_id WHERE ur.user_id=u.id AND r.name<>'admin')`).Scan(&n)
	return
}

// CreateUser inserts a user with a role and a default dashboard.
func (s *Store) CreateUser(ctx context.Context, login, hash, role string) (*User, error) {
	u, err := s.CreateAdmin(ctx, login, hash)
	if err != nil || role == RoleAdmin {
		return u, err
	}
	if err := s.SetUserAccess(ctx, u.ID, role, Access{}); err != nil {
		_ = s.DeleteUser(ctx, u.ID)
		return nil, err
	}
	return u, nil
}

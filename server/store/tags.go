package store

import "context"

// ServerTags returns the tags of a server in the order they were given (empty when there are none).
func (s *Store) ServerTags(ctx context.Context, serverID string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT tag FROM server_tags WHERE server_id=$1 ORDER BY position, tag`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetServerTags replaces the tags of a server; ErrNotFound when there is no such server.
func (s *Store) SetServerTags(ctx context.Context, serverID string, tags []string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var one int
	if err := tx.QueryRow(ctx, `SELECT 1 FROM servers WHERE id=$1 FOR UPDATE`, serverID).Scan(&one); err != nil {
		return notFound(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM server_tags WHERE server_id=$1`, serverID); err != nil {
		return err
	}
	for i, t := range tags {
		if _, err := tx.Exec(ctx, `INSERT INTO server_tags (server_id, tag, position) VALUES ($1,$2,$3)`, serverID, t, i); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

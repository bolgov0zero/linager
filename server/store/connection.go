package store

import "context"

// ServerPort is what the connection settings need to know about a registered agent.
type ServerPort struct {
	ID       string
	Name     string
	Version  string
	LastPort int
}

// SetServerPort remembers the agent port a server connected to.
func (s *Store) SetServerPort(ctx context.Context, id string, port int) error {
	_, err := s.Pool.Exec(ctx, `UPDATE servers SET last_port=$2 WHERE id=$1 AND last_port<>$2`, id, port)
	return err
}

// ServerPorts lists every registered agent with its version and the port it used last (0: never connected since the
// column exists).
func (s *Store) ServerPorts(ctx context.Context) ([]ServerPort, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text, name, agent_version, last_port FROM servers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ServerPort
	for rows.Next() {
		var sp ServerPort
		if err := rows.Scan(&sp.ID, &sp.Name, &sp.Version, &sp.LastPort); err != nil {
			return nil, err
		}
		out = append(out, sp)
	}
	return out, rows.Err()
}

// DeleteSetting removes a setting.
func (s *Store) DeleteSetting(ctx context.Context, key string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM settings WHERE key=$1`, key)
	return err
}

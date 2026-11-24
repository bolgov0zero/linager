package store

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// MaxTagsTotal is how many different tags the registry holds.
const MaxTagsTotal = 200

// DefaultTagColor is the colour of a tag that nobody coloured.
const DefaultTagColor = "gray"

// TagColors is the palette a tag can have. The application draws every key in both themes; there is no purple on purpose
// (the style of the application has none). The order is the order of the colour picker.
var TagColors = []string{
	"gray", "slate", "stone", "red", "rose", "pink", "orange", "amber",
	"yellow", "lime", "green", "emerald", "teal", "cyan", "sky", "blue",
}

// ValidTagColor reports whether the key is in the palette.
func ValidTagColor(c string) bool {
	for _, k := range TagColors {
		if k == c {
			return true
		}
	}
	return false
}

// ErrTagExists is returned when a tag with the same name (case does not matter) already exists.
var ErrTagExists = errors.New("tag name already in use")

// ErrTooManyTags is returned when the registry is full.
var ErrTooManyTags = errors.New("too many tags")

// Tag is a registry entry with how often it is used.
type Tag struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
	// Servers is how many servers carry the tag, Users how many users were given access by it.
	Servers int `json:"servers"`
	Users   int `json:"users"`
}

// TagRef is a tag as a server shows it.
type TagRef struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

func tagErr(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return ErrTagExists
	}
	return err
}

// Tags lists the registry by name.
func (s *Store) Tags(ctx context.Context) ([]Tag, error) {
	rows, err := s.Pool.Query(ctx, `SELECT t.id, t.name, t.color,
		(SELECT count(*) FROM server_tags st WHERE st.tag_id=t.id),
		(SELECT count(*) FROM user_tags ut WHERE ut.tag_id=t.id)
		FROM tags t ORDER BY lower(t.name), t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Tag{}
	for rows.Next() {
		var t Tag
		if err := rows.Scan(&t.ID, &t.Name, &t.Color, &t.Servers, &t.Users); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// TagCreate adds a tag to the registry.
func (s *Store) TagCreate(ctx context.Context, name, color string) (Tag, error) {
	t := Tag{Name: name, Color: color}
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM tags`).Scan(&n); err != nil {
		return t, err
	}
	if n >= MaxTagsTotal {
		return t, ErrTooManyTags
	}
	err := s.Pool.QueryRow(ctx, `INSERT INTO tags (name, color) VALUES ($1,$2) RETURNING id`, name, color).Scan(&t.ID)
	return t, tagErr(err)
}

// TagUpdate changes the name and/or the colour of a tag (nil keeps the value) and returns the tag as it was;
// ErrNotFound when there is no such tag, ErrTagExists when the new name is taken by another tag.
func (s *Store) TagUpdate(ctx context.Context, id int64, name, color *string) (Tag, error) {
	var old Tag
	old.ID = id
	err := s.Pool.QueryRow(ctx, `UPDATE tags n SET name=COALESCE($2, o.name), color=COALESCE($3, o.color)
		FROM tags o WHERE n.id=o.id AND n.id=$1 RETURNING o.name, o.color`, id, name, color).Scan(&old.Name, &old.Color)
	if err != nil {
		if e := notFound(err); errors.Is(e, ErrNotFound) {
			return old, e
		}
		return old, tagErr(err)
	}
	return old, nil
}

// TagDelete removes a tag from the registry, from every server and from every grant, and returns it with the numbers
// it had; ErrNotFound when there is no such tag.
func (s *Store) TagDelete(ctx context.Context, id int64) (Tag, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Tag{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	t := Tag{ID: id}
	err = tx.QueryRow(ctx, `SELECT name, color,
		(SELECT count(*) FROM server_tags WHERE tag_id=$1), (SELECT count(*) FROM user_tags WHERE tag_id=$1)
		FROM tags WHERE id=$1 FOR UPDATE`, id).Scan(&t.Name, &t.Color, &t.Servers, &t.Users)
	if err != nil {
		return t, notFound(err)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM tags WHERE id=$1`, id); err != nil {
		return t, err
	}
	return t, tx.Commit(ctx)
}

// ServerTags returns the tags of a server in the order they were given (empty when there are none).
func (s *Store) ServerTags(ctx context.Context, serverID string) ([]TagRef, error) {
	rows, err := s.Pool.Query(ctx, `SELECT t.id, t.name, t.color FROM server_tags st JOIN tags t ON t.id=st.tag_id
		WHERE st.server_id=$1 ORDER BY st.position, lower(t.name)`, serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TagRef{}
	for rows.Next() {
		var t TagRef
		if err := rows.Scan(&t.ID, &t.Name, &t.Color); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// SetServerTags replaces the tags of a server by names (a name the registry does not know yet is added to it, with the
// default colour). It returns the tags now on the server and how many users hold a grant by a tag that was put on or taken
// off, because their access to this server changed with it. ErrNotFound when there is no such server.
func (s *Store) SetServerTags(ctx context.Context, serverID string, names []string) ([]TagRef, int, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var one int
	if err := tx.QueryRow(ctx, `SELECT 1 FROM servers WHERE id=$1 FOR UPDATE`, serverID).Scan(&one); err != nil {
		return nil, 0, notFound(err)
	}
	before := map[int64]bool{}
	rows, err := tx.Query(ctx, `SELECT tag_id FROM server_tags WHERE server_id=$1`, serverID)
	if err != nil {
		return nil, 0, err
	}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, 0, err
		}
		before[id] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	ids := make([]int64, 0, len(names))
	for _, n := range names {
		id, err := tagIDByName(ctx, tx, n)
		if err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
	}
	if _, err := tx.Exec(ctx, `DELETE FROM server_tags WHERE server_id=$1`, serverID); err != nil {
		return nil, 0, err
	}
	now := map[int64]bool{}
	for i, id := range ids {
		if now[id] {
			continue
		}
		now[id] = true
		if _, err := tx.Exec(ctx, `INSERT INTO server_tags (server_id, tag_id, position) VALUES ($1,$2,$3)`, serverID, id, i); err != nil {
			return nil, 0, err
		}
	}
	changed := []int64{}
	for id := range before {
		if !now[id] {
			changed = append(changed, id)
		}
	}
	for id := range now {
		if !before[id] {
			changed = append(changed, id)
		}
	}
	users := 0
	if len(changed) > 0 {
		if err := tx.QueryRow(ctx, `SELECT count(DISTINCT user_id) FROM user_tags WHERE tag_id = ANY($1)`, changed).Scan(&users); err != nil {
			return nil, 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, 0, err
	}
	out, err := s.ServerTags(ctx, serverID)
	return out, users, err
}

// tagIDByName finds a tag by name (case does not matter) or adds it to the registry.
func tagIDByName(ctx context.Context, tx pgx.Tx, name string) (int64, error) {
	var id int64
	err := tx.QueryRow(ctx, `SELECT id FROM tags WHERE lower(name)=lower($1)`, name).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, err
	}
	var n int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM tags`).Scan(&n); err != nil {
		return 0, err
	}
	if n >= MaxTagsTotal {
		return 0, ErrTooManyTags
	}
	err = tx.QueryRow(ctx, `INSERT INTO tags (name, color) VALUES ($1,$2) ON CONFLICT (lower(name)) DO NOTHING RETURNING id`, name, DefaultTagColor).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) { // another request added it a moment ago
		err = tx.QueryRow(ctx, `SELECT id FROM tags WHERE lower(name)=lower($1)`, name).Scan(&id)
	}
	return id, err
}

package store

import (
	"context"
	"errors"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// MaxGroupDepth is how many levels of folders there can be (a folder of the top level is level 1).
const MaxGroupDepth = 8

// MaxGroups is how many folders there can be.
const MaxGroups = 200

// ErrGroupExists is returned when a folder with the same name (case-insensitive) already exists in the same parent.
var ErrGroupExists = errors.New("folder name already in use here")

// ErrGroupCycle is returned when a folder would be put into itself or into one of its own subfolders.
var ErrGroupCycle = errors.New("a folder cannot be put into itself")

// ErrGroupDepth is returned when the folders would go deeper than MaxGroupDepth.
var ErrGroupDepth = errors.New("too many levels of folders")

// ErrGroupBefore is returned when the folder to put a folder before is not in the wanted parent.
var ErrGroupBefore = errors.New("the folder to put before is not in this parent")

// NameClash is ErrGroupExists that names the folder in the way.
type NameClash struct{ Name string }

func (e *NameClash) Error() string        { return ErrGroupExists.Error() + ": " + e.Name }
func (e *NameClash) Is(target error) bool { return target == ErrGroupExists }

// Group is a folder of servers; ParentID is nil for a folder of the top level.
type Group struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	ParentID *int64 `json:"parent_id"`
}

func groupErr(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) && pe.Code == "23505" {
		return ErrGroupExists
	}
	return err
}

// Groups lists the folders in the order the administrator arranged them (the order inside a parent is the order of the list).
func (s *Store) Groups(ctx context.Context) ([]Group, error) {
	return groupList(ctx, s.Pool)
}

func groupList(ctx context.Context, q dbq) ([]Group, error) {
	rows, err := q.Query(ctx, `SELECT id, name, parent_id FROM server_groups ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Group{}
	for rows.Next() {
		var g Group
		if err := rows.Scan(&g.ID, &g.Name, &g.ParentID); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// tree is the folders in memory: the checks of a move are done on it before anything is written.
type tree struct {
	parent map[int64]*int64
	name   map[int64]string
	order  []int64 // the order of the list
}

func loadTree(ctx context.Context, q dbq) (*tree, error) {
	list, err := groupList(ctx, q)
	if err != nil {
		return nil, err
	}
	t := &tree{parent: map[int64]*int64{}, name: map[int64]string{}}
	for _, g := range list {
		t.parent[g.ID] = g.ParentID
		t.name[g.ID] = g.Name
		t.order = append(t.order, g.ID)
	}
	return t, nil
}

func (t *tree) has(id int64) bool { _, ok := t.parent[id]; return ok }

// within reports whether id is anc or lies under it.
func (t *tree) within(id, anc int64) bool {
	for cur, n := &id, 0; cur != nil && n <= len(t.parent); n++ {
		if *cur == anc {
			return true
		}
		cur = t.parent[*cur]
	}
	return false
}

// depth is the level of a folder (a folder of the top level is 1).
func (t *tree) depth(id int64) int {
	d := 0
	for cur := &id; cur != nil && d <= len(t.parent); d++ {
		cur = t.parent[*cur]
	}
	return d
}

// height is the number of levels of the subtree of a folder, the folder itself included.
func (t *tree) height(id int64) int {
	h := 1
	for _, c := range t.order {
		if p := t.parent[c]; p != nil && *p == id {
			if x := 1 + t.height(c); x > h {
				h = x
			}
		}
	}
	return h
}

// children lists the folders of a parent in order (nil = the top level).
func (t *tree) children(parent *int64) []int64 {
	var out []int64
	for _, c := range t.order {
		p := t.parent[c]
		if (p == nil && parent == nil) || (p != nil && parent != nil && *p == *parent) {
			out = append(out, c)
		}
	}
	return out
}

func (t *tree) clash(name string, parent *int64, except int64) (string, bool) {
	for _, c := range t.children(parent) {
		if c != except && lower(t.name[c]) == lower(name) {
			return t.name[c], true
		}
	}
	return "", false
}

func lower(s string) string { return strings.ToLower(s) }

// lockTree serialises the changes of the folders, so two moves cannot make a cycle together.
func lockTree(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(7201)`)
	return err
}

// setPositions writes the order of the folders of one parent as 1..n.
func setPositions(ctx context.Context, tx pgx.Tx, ids []int64) error {
	for i, id := range ids {
		if _, err := tx.Exec(ctx, `UPDATE server_groups SET position=$2 WHERE id=$1`, id, i+1); err != nil {
			return err
		}
	}
	return nil
}

// GroupCreate adds a folder under parent (nil = the top level), last among its folders.
func (s *Store) GroupCreate(ctx context.Context, name string, parent *int64) (Group, error) {
	g := Group{Name: name, ParentID: parent}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return g, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockTree(ctx, tx); err != nil {
		return g, err
	}
	t, err := loadTree(ctx, tx)
	if err != nil {
		return g, err
	}
	if len(t.order) >= MaxGroups {
		return g, ErrTooManyGroups
	}
	if parent != nil {
		if !t.has(*parent) {
			return g, ErrNotFound
		}
		if t.depth(*parent)+1 > MaxGroupDepth {
			return g, ErrGroupDepth
		}
	}
	if n, bad := t.clash(name, parent, 0); bad {
		return g, &NameClash{Name: n}
	}
	err = tx.QueryRow(ctx, `INSERT INTO server_groups (name, parent_id, position) VALUES ($1,$2,
		(SELECT COALESCE(MAX(position), 0) + 1 FROM server_groups)) RETURNING id`, name, parent).Scan(&g.ID)
	if err != nil {
		return g, groupErr(err)
	}
	return g, tx.Commit(ctx)
}

// ErrTooManyGroups is returned when the limit of folders is reached.
var ErrTooManyGroups = errors.New("too many folders")

// GroupRename renames a folder; ErrNotFound if it does not exist, ErrGroupExists when its parent has a folder of this name.
func (s *Store) GroupRename(ctx context.Context, id int64, name string) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE server_groups SET name=$2 WHERE id=$1`, id, name)
	if err != nil {
		return groupErr(err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// AccessChange is what a change of the folders does to the access of one user: the servers they stop and start to see.
type AccessChange struct {
	UserID int64    `json:"user_id"`
	Login  string   `json:"login"`
	Role   string   `json:"role"`
	Lose   []string `json:"lose"` // names, the first ones only
	Gain   []string `json:"gain"`
	LoseN  int      `json:"lose_n"` // how many in all
	GainN  int      `json:"gain_n"`
}

const impactNames = 20

// accessSnapshot returns the servers every operator and auditor sees now.
func accessSnapshot(ctx context.Context, tx pgx.Tx) (map[int64]map[string]bool, map[int64][2]string, error) {
	rows, err := tx.Query(ctx, `SELECT u.id, u.login, r.name FROM users u JOIN user_roles ur ON ur.user_id=u.id JOIN roles r ON r.id=ur.role_id WHERE r.name<>'admin' ORDER BY u.id`)
	if err != nil {
		return nil, nil, err
	}
	who := map[int64][2]string{}
	var ids []int64
	for rows.Next() {
		var id int64
		var login, role string
		if err := rows.Scan(&id, &login, &role); err != nil {
			rows.Close()
			return nil, nil, err
		}
		who[id] = [2]string{login, role}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	snap := map[int64]map[string]bool{}
	for _, id := range ids {
		m, err := allowedServerIDs(ctx, tx, id)
		if err != nil {
			return nil, nil, err
		}
		snap[id] = m
	}
	return snap, who, nil
}

// diffAccess compares two snapshots and names the servers; only users whose access changed are listed.
func diffAccess(ctx context.Context, tx pgx.Tx, before, after map[int64]map[string]bool, who map[int64][2]string) ([]AccessChange, error) {
	names := map[string]string{}
	rows, err := tx.Query(ctx, `SELECT id::text, name FROM servers`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id, name string
		if err := rows.Scan(&id, &name); err != nil {
			rows.Close()
			return nil, err
		}
		names[id] = name
	}
	rows.Close()
	out := []AccessChange{}
	uids := make([]int64, 0, len(before))
	for id := range before {
		uids = append(uids, id)
	}
	sort.Slice(uids, func(i, j int) bool { return uids[i] < uids[j] })
	for _, uid := range uids {
		var lose, gain []string
		for id := range before[uid] {
			if !after[uid][id] {
				lose = append(lose, names[id])
			}
		}
		for id := range after[uid] {
			if !before[uid][id] {
				gain = append(gain, names[id])
			}
		}
		if len(lose) == 0 && len(gain) == 0 {
			continue
		}
		sort.Strings(lose)
		sort.Strings(gain)
		c := AccessChange{UserID: uid, Login: who[uid][0], Role: who[uid][1], LoseN: len(lose), GainN: len(gain), Lose: lose, Gain: gain}
		if len(c.Lose) > impactNames {
			c.Lose = c.Lose[:impactNames]
		}
		if len(c.Gain) > impactNames {
			c.Gain = c.Gain[:impactNames]
		}
		if c.Lose == nil {
			c.Lose = []string{}
		}
		if c.Gain == nil {
			c.Gain = []string{}
		}
		out = append(out, c)
	}
	return out, nil
}

// withImpact runs a change inside a transaction and reports what it did to the access of the users. With dry the change is
// rolled back: the answer is a preview.
func (s *Store) withImpact(ctx context.Context, dry bool, change func(tx pgx.Tx) error) ([]AccessChange, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := lockTree(ctx, tx); err != nil {
		return nil, err
	}
	before, who, err := accessSnapshot(ctx, tx)
	if err != nil {
		return nil, err
	}
	if err := change(tx); err != nil {
		return nil, err
	}
	after, _, err := accessSnapshot(ctx, tx)
	if err != nil {
		return nil, err
	}
	imp, err := diffAccess(ctx, tx, before, after, who)
	if err != nil {
		return nil, err
	}
	if dry {
		return imp, nil
	}
	return imp, tx.Commit(ctx)
}

// GroupMove puts a folder under parent (nil = the top level), before the folder `before` of that parent (nil = last). The
// answer lists the users whose access it changes. With dry nothing is written.
// Errors: ErrNotFound, ErrGroupCycle, ErrGroupDepth, ErrGroupBefore, and ErrGroupExists (a *NameClash) for a name taken there.
func (s *Store) GroupMove(ctx context.Context, id int64, parent, before *int64, dry bool) ([]AccessChange, error) {
	return s.withImpact(ctx, dry, func(tx pgx.Tx) error {
		t, err := loadTree(ctx, tx)
		if err != nil {
			return err
		}
		if !t.has(id) || (parent != nil && !t.has(*parent)) {
			return ErrNotFound
		}
		if parent != nil && t.within(*parent, id) {
			return ErrGroupCycle
		}
		top := 0
		if parent != nil {
			top = t.depth(*parent)
		}
		if top+t.height(id) > MaxGroupDepth {
			return ErrGroupDepth
		}
		if n, bad := t.clash(t.name[id], parent, id); bad {
			return &NameClash{Name: n}
		}
		sibs := []int64{}
		for _, c := range t.children(parent) {
			if c != id {
				sibs = append(sibs, c)
			}
		}
		at := len(sibs)
		if before != nil {
			if *before == id {
				return ErrGroupBefore
			}
			at = -1
			for i, c := range sibs {
				if c == *before {
					at = i
				}
			}
			if at < 0 {
				return ErrGroupBefore
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE server_groups SET parent_id=$2 WHERE id=$1`, id, parent); err != nil {
			return groupErr(err)
		}
		order := append(append(append([]int64{}, sibs[:at]...), id), sibs[at:]...)
		return setPositions(ctx, tx, order)
	})
}

// GroupDeleteResult says what a delete did (or, with dry, would do).
type GroupDeleteResult struct {
	Name       string
	Subfolders int // the folders directly in it, moved up
	Servers    int // the servers directly in it, moved up
	Grants     []string
	Impact     []AccessChange
}

// GroupDelete removes a folder. Its subfolders take its place in the parent (in the same spot of the order), its servers go to
// the parent (or to no folder), and the access given by this folder is gone. ErrGroupExists (a *NameClash) when a subfolder would
// meet a folder of the same name in the parent. With dry nothing is written.
func (s *Store) GroupDelete(ctx context.Context, id int64, dry bool) (GroupDeleteResult, error) {
	var res GroupDeleteResult
	imp, err := s.withImpact(ctx, dry, func(tx pgx.Tx) error {
		t, err := loadTree(ctx, tx)
		if err != nil {
			return err
		}
		if !t.has(id) {
			return ErrNotFound
		}
		res.Name = t.name[id]
		parent := t.parent[id]
		kids := t.children(&id)
		res.Subfolders = len(kids)
		for _, k := range kids {
			if n, bad := t.clash(t.name[k], parent, id); bad {
				return &NameClash{Name: n}
			}
		}
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM servers WHERE group_id=$1`, id).Scan(&res.Servers); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT u.login FROM user_groups g JOIN users u ON u.id=g.user_id WHERE g.group_id=$1 ORDER BY u.login`, id)
		if err != nil {
			return err
		}
		res.Grants = []string{}
		for rows.Next() {
			var l string
			if err := rows.Scan(&l); err != nil {
				rows.Close()
				return err
			}
			res.Grants = append(res.Grants, l)
		}
		rows.Close()
		// the children take the place of the deleted folder in the order of the parent
		var order []int64
		for _, c := range t.children(parent) {
			if c == id {
				order = append(order, kids...)
			} else {
				order = append(order, c)
			}
		}
		if _, err := tx.Exec(ctx, `UPDATE server_groups SET parent_id=$2 WHERE parent_id=$1`, id, parent); err != nil {
			return groupErr(err)
		}
		if _, err := tx.Exec(ctx, `UPDATE servers SET group_id=$2 WHERE group_id=$1`, id, parent); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE join_tokens SET group_id=$2 WHERE group_id=$1`, id, parent); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `DELETE FROM server_groups WHERE id=$1`, id); err != nil {
			return err
		}
		return setPositions(ctx, tx, order)
	})
	res.Impact = imp
	return res, err
}

// GroupName returns the name of a folder or ErrNotFound.
func (s *Store) GroupName(ctx context.Context, id int64) (string, error) {
	var name string
	err := s.Pool.QueryRow(ctx, `SELECT name FROM server_groups WHERE id=$1`, id).Scan(&name)
	return name, notFound(err)
}

// GroupPath returns the names of a folder and of the folders above it, joined by " / ", or ErrNotFound.
func (s *Store) GroupPath(ctx context.Context, id int64) (string, error) {
	t, err := loadTree(ctx, s.Pool)
	if err != nil {
		return "", err
	}
	if !t.has(id) {
		return "", ErrNotFound
	}
	path := ""
	for cur, n := &id, 0; cur != nil && n <= len(t.parent); n++ {
		if path == "" {
			path = t.name[*cur]
		} else {
			path = t.name[*cur] + " / " + path
		}
		cur = t.parent[*cur]
	}
	return path, nil
}

// SetServerGroup puts a server into a folder (nil = no folder).
func (s *Store) SetServerGroup(ctx context.Context, serverID string, groupID *int64) error {
	tag, err := s.Pool.Exec(ctx, `UPDATE servers SET group_id=$2 WHERE id=$1`, serverID, groupID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// MoveServerToGroup puts a server into a folder (nil = no folder) and lists the users whose access that changes. With dry
// nothing is written. ErrNotFound when the server or the folder does not exist.
func (s *Store) MoveServerToGroup(ctx context.Context, serverID string, groupID *int64, dry bool) ([]AccessChange, error) {
	return s.withImpact(ctx, dry, func(tx pgx.Tx) error {
		if groupID != nil {
			var one int
			if err := tx.QueryRow(ctx, `SELECT 1 FROM server_groups WHERE id=$1`, *groupID).Scan(&one); err != nil {
				return notFound(err)
			}
		}
		tag, err := tx.Exec(ctx, `UPDATE servers SET group_id=$2 WHERE id=$1`, serverID, groupID)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return ErrNotFound
		}
		return nil
	})
}

// ServerGroups maps server id to folder id for servers that are in a folder.
func (s *Store) ServerGroups(ctx context.Context) (map[string]int64, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id::text, group_id FROM servers WHERE group_id IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var id string
		var g int64
		if err := rows.Scan(&id, &g); err != nil {
			return nil, err
		}
		out[id] = g
	}
	return out, rows.Err()
}

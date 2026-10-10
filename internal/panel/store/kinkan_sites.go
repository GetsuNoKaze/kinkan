package store

import (
	"context"
	"database/sql"
	"errors"
	"sort"

	"github.com/jackc/pgx/v5/pgconn"

	"mikan/internal/site"
)

// Kinkan: node sites (migration kinkan/00001_node_sites.sql, ROADMAP item 2).

// Site is a stored site without its archive.
type Site struct {
	ID        int64   `json:"id"`
	Name      string  `json:"name"`
	Hash      string  `json:"hash"`
	Title     string  `json:"title"`
	Size      int64   `json:"size"`
	Files     int     `json:"files"`
	CreatedAt int64   `json:"created_at"`
	Nodes     []int64 `json:"nodes"` // the nodes that show it
}

// ErrSiteInUse: a node shows the site, so it stays.
var ErrSiteInUse = errors.New("site_in_use")

// AddSite keeps a checked site under a name. The same content uploaded again is the site
// already kept (its name stays); found says so.
func (s *Store) AddSite(ctx context.Context, name string, st *site.Site, now int64) (_ Site, found bool, _ error) {
	archive, err := st.Archive()
	if err != nil {
		return Site{}, false, err
	}
	var id int64
	err = s.DB.QueryRowContext(ctx, `INSERT INTO kinkan_sites (name, hash, title, size, files, archive, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7) ON CONFLICT (hash) DO NOTHING RETURNING id`,
		name, st.Hash, st.Title, st.Size, len(st.Files), archive, now).Scan(&id)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		found = true
		if err = s.DB.QueryRowContext(ctx, `SELECT id FROM kinkan_sites WHERE hash = $1`, st.Hash).Scan(&id); err != nil {
			return Site{}, false, err
		}
	case err != nil:
		return Site{}, false, err
	}
	got, err := s.GetSite(ctx, id)
	return got, found, err
}

// GetSite is one site; sql.ErrNoRows when there is none.
func (s *Store) GetSite(ctx context.Context, id int64) (Site, error) {
	sites, err := s.sites(ctx, `WHERE id = $1`, id)
	if err != nil {
		return Site{}, err
	}
	if len(sites) == 0 {
		return Site{}, sql.ErrNoRows
	}
	return sites[0], nil
}

// Sites are all kept sites, newest first.
func (s *Store) Sites(ctx context.Context) ([]Site, error) { return s.sites(ctx, ``) }

func (s *Store) sites(ctx context.Context, where string, args ...any) ([]Site, error) {
	rows, err := s.DB.QueryContext(ctx, `SELECT id, name, hash, title, size, files, created_at FROM kinkan_sites `+where+` ORDER BY id DESC`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Site
	index := map[int64]int{}
	for rows.Next() {
		var x Site
		if err := rows.Scan(&x.ID, &x.Name, &x.Hash, &x.Title, &x.Size, &x.Files, &x.CreatedAt); err != nil {
			return nil, err
		}
		x.Nodes = []int64{}
		index[x.ID] = len(out)
		out = append(out, x)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	shows, err := s.DB.QueryContext(ctx, `SELECT node_id, site_id FROM kinkan_node_sites`)
	if err != nil {
		return nil, err
	}
	defer shows.Close()
	for shows.Next() {
		var node, id int64
		if err := shows.Scan(&node, &id); err != nil {
			return nil, err
		}
		if i, ok := index[id]; ok {
			out[i].Nodes = append(out[i].Nodes, node)
		}
	}
	for i := range out {
		sort.Slice(out[i].Nodes, func(a, b int) bool { return out[i].Nodes[a] < out[i].Nodes[b] })
	}
	return out, shows.Err()
}

// SiteArchive is a site's packed files; sql.ErrNoRows when there is none.
func (s *Store) SiteArchive(ctx context.Context, id int64) (hash string, archive []byte, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT hash, archive FROM kinkan_sites WHERE id = $1`, id).Scan(&hash, &archive)
	return hash, archive, err
}

// DeleteSite forgets a site no node shows: ErrSiteInUse when one does, sql.ErrNoRows when
// there is no such site.
func (s *Store) DeleteSite(ctx context.Context, id int64) error {
	var shown bool
	if err := s.DB.QueryRowContext(ctx, `SELECT EXISTS (SELECT 1 FROM kinkan_node_sites WHERE site_id = $1)`, id).Scan(&shown); err != nil {
		return err
	}
	if shown {
		return ErrSiteInUse
	}
	res, err := s.DB.ExecContext(ctx, `DELETE FROM kinkan_sites WHERE id = $1`, id)
	if err != nil {
		if isForeignKey(err) { // a node took it between the check and the delete
			return ErrSiteInUse
		}
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// SetNodeSite makes a node show a site; siteID 0 makes it show none. sql.ErrNoRows when
// the node or the site is not there.
func (s *Store) SetNodeSite(ctx context.Context, nodeID, siteID int64) error {
	if siteID == 0 {
		_, err := s.DB.ExecContext(ctx, `DELETE FROM kinkan_node_sites WHERE node_id = $1`, nodeID)
		return err
	}
	_, err := s.DB.ExecContext(ctx, `INSERT INTO kinkan_node_sites (node_id, site_id) VALUES ($1, $2)
		ON CONFLICT (node_id) DO UPDATE SET site_id = EXCLUDED.site_id`, nodeID, siteID)
	if isForeignKey(err) {
		return sql.ErrNoRows
	}
	return err
}

// NodeSite is the site a node shows: its id and hash, ok false when none.
func (s *Store) NodeSite(ctx context.Context, nodeID int64) (id int64, hash string, ok bool, err error) {
	err = s.DB.QueryRowContext(ctx, `SELECT s.id, s.hash FROM kinkan_node_sites n JOIN kinkan_sites s ON s.id = n.site_id WHERE n.node_id = $1`, nodeID).Scan(&id, &hash)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	return id, hash, err == nil, err
}

// isForeignKey tells a foreign_key_violation (23503).
func isForeignKey(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && pe.Code == "23503"
}

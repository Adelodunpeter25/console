// Persistent cross-session memory. Port of agent/src/memory/: one
// `memories` table per database file, deterministic keyword/tag recall
// (no embeddings), and a registry resolving (scope, project) to stores.
package memory

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
	_ "modernc.org/sqlite"
)

type Scope string

const (
	ScopeProject Scope = "project"
	ScopeGlobal  Scope = "global"
)

// Entry is one stored memory.
type Entry struct {
	ID        string   `json:"id"`
	Scope     Scope    `json:"scope"`
	Content   string   `json:"content"`
	Tags      []string `json:"tags"`
	CreatedAt int64    `json:"createdAt"`
	UpdatedAt int64    `json:"updatedAt"`
}

// Match pairs an entry with its recall score.
type Match struct {
	Entry Entry
	Score int
}

// Query filters recall runs.
type Query struct {
	Text string
	Tags []string
}

// Store is one memories database file (project or global).
type Store struct {
	db    *sql.DB
	scope Scope
}

// OpenStore opens (creating parents) a memories database file.
func OpenStore(path string, scope Scope) (*Store, error) {
	if dir := filepath.Dir(path); dir != "." && dir != "/" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS memories (
			id TEXT PRIMARY KEY,
			scope TEXT NOT NULL,
			content TEXT NOT NULL,
			tags TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_memories_scope ON memories(scope);
	`); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db, scope: scope}, nil
}

// Close releases the database connection.
func (s *Store) Close() error { return s.db.Close() }

// MemoryStore saves a new entry.
func (s *Store) Store(content string, tags []string) (Entry, error) {
	now := utils.NowMillis()
	entry := Entry{ID: utils.RandomID(), Scope: s.scope, Content: content, Tags: tags, CreatedAt: now, UpdatedAt: now}
	raw, err := json.Marshal(tagsOrEmpty(tags))
	if err != nil {
		return Entry{}, err
	}
	if _, err := s.db.Exec(
		`INSERT INTO memories (id, scope, content, tags, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		entry.ID, string(entry.Scope), entry.Content, string(raw), entry.CreatedAt, entry.UpdatedAt); err != nil {
		return Entry{}, err
	}
	entry.Tags = tagsOrEmpty(tags)
	return entry, nil
}

// Get returns one entry by id, or nil when missing.
func (s *Store) Get(id string) (*Entry, error) {
	row := s.db.QueryRow(`SELECT id, scope, content, tags, created_at, updated_at FROM memories WHERE id = ?`, id)
	entry, err := scanEntry(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return entry, nil
}

// List returns entries newest-first, optionally filtered by tag overlap.
func (s *Store) List(tags []string) ([]Entry, error) {
	rows, err := s.db.Query(`SELECT id, scope, content, tags, created_at, updated_at FROM memories ORDER BY updated_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		entry, err := scanRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(tags) == 0 {
		return out, nil
	}
	wanted := make(map[string]bool, len(tags))
	for _, t := range tags {
		wanted[t] = true
	}
	filtered := out[:0]
	for _, e := range out {
		for _, t := range e.Tags {
			if wanted[t] {
				filtered = append(filtered, e)
				break
			}
		}
	}
	return filtered, nil
}

// Update patches content and/or tags, returning false when missing.
func (s *Store) Update(id string, content *string, tags []string, hasTags bool) (*Entry, error) {
	existing, err := s.Get(id)
	if err != nil || existing == nil {
		return nil, err
	}
	if content != nil {
		existing.Content = *content
	}
	if hasTags {
		existing.Tags = tagsOrEmpty(tags)
	}
	existing.UpdatedAt = utils.NowMillis()
	raw, err := json.Marshal(existing.Tags)
	if err != nil {
		return nil, err
	}
	if _, err := s.db.Exec(`UPDATE memories SET content = ?, tags = ?, updated_at = ? WHERE id = ?`,
		existing.Content, string(raw), existing.UpdatedAt, id); err != nil {
		return nil, err
	}
	return existing, nil
}

// Remove deletes one entry, returning false when missing.
func (s *Store) Remove(id string) (bool, error) {
	res, err := s.db.Exec(`DELETE FROM memories WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

func tagsOrEmpty(tags []string) []string {
	if tags == nil {
		return []string{}
	}
	return tags
}

func scanEntry(row *sql.Row) (*Entry, error) {
	var e Entry
	var scope, tags string
	if err := row.Scan(&e.ID, &scope, &e.Content, &tags, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	e.Scope = Scope(scope)
	if err := json.Unmarshal([]byte(tags), &e.Tags); err != nil {
		return nil, fmt.Errorf("decode memory tags: %w", err)
	}
	return &e, nil
}

func scanRows(rows *sql.Rows) (*Entry, error) {
	var e Entry
	var scope, tags string
	if err := rows.Scan(&e.ID, &scope, &e.Content, &tags, &e.CreatedAt, &e.UpdatedAt); err != nil {
		return nil, err
	}
	e.Scope = Scope(scope)
	if err := json.Unmarshal([]byte(tags), &e.Tags); err != nil {
		return nil, fmt.Errorf("decode memory tags: %w", err)
	}
	return &e, nil
}

// RecallMemories scores entries by tag overlap (weight 3) and substring
// text match (weight 1), returning positives sorted by score, capped.
func RecallMemories(entries []Entry, query Query, limit int) []Match {
	if limit <= 0 {
		limit = 10
	}
	wanted := make(map[string]bool, len(query.Tags))
	for _, t := range query.Tags {
		wanted[t] = true
	}
	text := strings.ToLower(strings.TrimSpace(query.Text))
	var scored []Match
	for _, entry := range entries {
		score := 0
		for _, t := range entry.Tags {
			if wanted[t] {
				score += 3
			}
		}
		if text != "" && strings.Contains(strings.ToLower(entry.Content), text) {
			score++
		}
		if score > 0 {
			scored = append(scored, Match{Entry: entry, Score: score})
		}
	}
	sort.SliceStable(scored, func(i, j int) bool { return scored[i].Score > scored[j].Score })
	if len(scored) > limit {
		scored = scored[:limit]
	}
	return scored
}

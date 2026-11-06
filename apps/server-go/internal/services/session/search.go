// Searching what people said. One function does the work; the agent's
// console.sessions.search calls it today, and a future Cmd+F in the chat view
// calls the same one, so there is a single definition of "a match".
//
// Only words are searched: what the user wrote, what the assistant replied, and
// one line per tool call. Tool output, thinking, images and server-injected
// messages are never searched, because nobody wrote them to be read.
//
// A session whose old messages have not yet been split has no searchable text
// for them, so search converts a session on the way through (cheap, safe, and
// the same conversion the first read would have done) rather than silently
// skipping history.
package session

import (
	"fmt"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

const (
	// DefaultSearchLimit and MaxSearchLimit bound the hits returned in total.
	DefaultSearchLimit = 20
	MaxSearchLimit     = 100
	// DefaultPerSession keeps one chatty session from filling the whole result.
	DefaultPerSession = 3
	// MaxSearchSessions bounds how many sessions one search will open.
	MaxSearchSessions = 200
	// MaxQueryWords bounds how many words a query may contain.
	MaxQueryWords = 8
	// snippetRadius is how many characters of context surround a match.
	snippetRadius = 60
)

// SearchScope says which sessions to look in. SessionID wins; otherwise
// Cwd, then ProjectID; with none of them, every session.
type SearchScope struct {
	SessionID string
	ProjectID string
	Cwd       string
	// Exclude lists sessions to skip, such as the one the caller is running in.
	Exclude []string
	// IncludeDeleted also searches soft-deleted (trashed) sessions.
	IncludeDeleted bool
}

// SearchOptions tunes a search.
type SearchOptions struct {
	// Roles limits which messages count: "user", "assistant". Empty means both.
	Roles []string
	// Limit caps total hits; PerSession caps hits from one session.
	Limit      int
	PerSession int
}

// Match locates one occurrence of the query inside a message's text, in
// characters (not bytes), so a UI can highlight exactly that span.
type Match struct {
	Start  int `json:"start"`
	Length int `json:"length"`
}

// SearchHit is one message that matched.
type SearchHit struct {
	SessionID    string `json:"session_id"`
	SessionTitle string `json:"session_title"`
	MessageID    string `json:"message_id"`
	Role         string `json:"role"`
	CreatedAt    int64  `json:"created_at"`
	Snippet      string `json:"snippet"`
	// SessionDeleted is true when the session the hit came from is
	// soft-deleted (in the trash); such a session is only readable
	// through a reader that includes deleted sessions.
	SessionDeleted bool `json:"session_deleted,omitempty"`
	// Matches are the occurrences within the message's full searchable text.
	Matches []Match `json:"matches"`
	// SnippetStart is where the snippet begins within that text, in characters,
	// so Matches can be shifted into the snippet.
	SnippetStart int `json:"snippet_start"`
}

// SearchResult is the outcome of a search.
type SearchResult struct {
	Query string      `json:"query"`
	Hits  []SearchHit `json:"hits"`
	// SessionsSearched counts sessions actually read.
	SessionsSearched int `json:"sessions_searched"`
	// Truncated is true when the search stopped early (the hit limit, or more
	// sessions in scope than one search will open), so "no more results" is not
	// the same as "nothing more exists".
	Truncated bool `json:"truncated"`
	// Note explains a truncation in words.
	Note string `json:"note,omitempty"`
}

// ParseQuery splits a query into lowercase words. Every word must appear in a
// message for it to match.
func ParseQuery(q string) ([]string, error) {
	fields := strings.Fields(strings.ToLower(q))
	if len(fields) == 0 {
		return nil, fmt.Errorf("query is empty")
	}
	if len(fields) > MaxQueryWords {
		return nil, fmt.Errorf("query has %d words; the limit is %d", len(fields), MaxQueryWords)
	}
	return fields, nil
}

// Search finds messages whose words contain every word of query.
func (s *Service) Search(scope SearchScope, query string, opts SearchOptions) (*SearchResult, error) {
	words, err := ParseQuery(query)
	if err != nil {
		return nil, err
	}
	limit := opts.Limit
	if limit <= 0 {
		limit = DefaultSearchLimit
	}
	if limit > MaxSearchLimit {
		limit = MaxSearchLimit
	}
	perSession := opts.PerSession
	if perSession <= 0 {
		perSession = DefaultPerSession
	}
	roles, err := normalizeRoles(opts.Roles)
	if err != nil {
		return nil, err
	}

	headers, err := s.searchScope(scope)
	if err != nil {
		return nil, err
	}
	skip := make(map[string]bool, len(scope.Exclude))
	for _, id := range scope.Exclude {
		skip[id] = true
	}

	res := &SearchResult{Query: strings.Join(words, " ")}
	var candidates []types.SessionHeader
	for _, h := range headers {
		if !skip[h.ID] {
			candidates = append(candidates, h)
		}
	}
	if len(candidates) > MaxSearchSessions {
		candidates = candidates[:MaxSearchSessions]
		res.Truncated = true
		res.Note = fmt.Sprintf("only the %d most recent sessions in scope were searched", MaxSearchSessions)
	}

	for _, h := range candidates {
		if len(res.Hits) >= limit {
			res.Truncated = true
			res.Note = "stopped at the result limit; narrow the query or raise limit to see more"
			break
		}
		hits, err := s.searchSession(h, words, roles, perSession)
		if err != nil {
			// One unreadable session must not hide the others.
			continue
		}
		res.SessionsSearched++
		for _, hit := range hits {
			hit.SessionDeleted = h.DeletedAt != nil
			if len(res.Hits) >= limit {
				res.Truncated = true
				res.Note = "stopped at the result limit; narrow the query or raise limit to see more"
				break
			}
			res.Hits = append(res.Hits, hit)
		}
	}
	// Newest message first across sessions.
	sort.SliceStable(res.Hits, func(i, j int) bool { return res.Hits[i].CreatedAt > res.Hits[j].CreatedAt })
	return res, nil
}

func normalizeRoles(roles []string) ([]string, error) {
	if len(roles) == 0 {
		return []string{"user", "assistant"}, nil
	}
	seen := map[string]bool{}
	var out []string
	for _, r := range roles {
		r = strings.ToLower(strings.TrimSpace(r))
		if r != "user" && r != "assistant" {
			return nil, fmt.Errorf(`role %q is not searchable; use "user" or "assistant"`, r)
		}
		if !seen[r] {
			seen[r] = true
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *Service) searchScope(scope SearchScope) ([]types.SessionHeader, error) {
	if scope.SessionID != "" {
		h, err := s.HeaderIncludingDeleted(scope.SessionID)
		if err != nil {
			return nil, err
		}
		if h == nil || (h.DeletedAt != nil && !scope.IncludeDeleted) {
			return nil, fmt.Errorf("session %q not found", scope.SessionID)
		}
		return []types.SessionHeader{*h}, nil
	}
	return s.ListFiltered(ListFilter{Cwd: scope.Cwd, ProjectID: scope.ProjectID, IncludeDeleted: scope.IncludeDeleted})
}

// searchSession finds up to perSession hits in one session, newest first.
func (s *Service) searchSession(h types.SessionHeader, words, roles []string, perSession int) ([]SearchHit, error) {
	projectID := projectIDFrom(h.ProjectID)
	conn, err := s.manager.Session(h.ID, projectID)
	if err != nil {
		return nil, err
	}
	// Old messages have no searchable text until they are split.
	s.ensureBackfilled(h.ID, projectID, conn)

	roleMarks := strings.TrimSuffix(strings.Repeat("?,", len(roles)), ",")
	args := make([]any, 0, len(roles))
	for _, r := range roles {
		args = append(args, r)
	}
	// Matching happens in Go, not SQL: SQLite's lower() folds only ASCII, so a
	// SQL filter would discard "ÉCOLE" before a search for "école" ever saw it.
	// The text column holds only words (about 1 MB for a whole real history), so
	// streaming it newest first and stopping at perSession hits is cheap.
	rows, err := conn.Query(
		`SELECT id, role, created_at, text FROM messages
		 WHERE role IN (`+roleMarks+`) AND text IS NOT NULL AND text != ''
		 ORDER BY created_at DESC, rowid DESC`, args...)
	if err != nil {
		return nil, err
	}
	// Close before returning, and never run another query on this single
	// connection while the rows are open.
	defer rows.Close()

	var hits []SearchHit
	for rows.Next() {
		var id, role, text string
		var created int64
		if err := rows.Scan(&id, &role, &created, &text); err != nil {
			return nil, err
		}
		matches := findMatches(text, words)
		if matches == nil {
			continue
		}
		snippet, start := makeSnippet(text, matches)
		hits = append(hits, SearchHit{
			SessionID: h.ID, SessionTitle: h.Title, MessageID: id, Role: role,
			CreatedAt: created, Snippet: snippet, SnippetStart: start, Matches: matches,
		})
		if len(hits) >= perSession {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return hits, nil
}

// findMatches returns every occurrence of every word in text, in characters,
// or nil when any word is absent. Matching is case-insensitive using Unicode
// simple folding, so "É" matches "é".
func findMatches(text string, words []string) []Match {
	runes := []rune(text)
	folded := make([]rune, len(runes))
	for i, r := range runes {
		folded[i] = unicode.ToLower(r)
	}
	var out []Match
	for _, w := range words {
		wr := []rune(w)
		for i := range wr {
			wr[i] = unicode.ToLower(wr[i])
		}
		found := false
		for i := 0; i+len(wr) <= len(folded); i++ {
			if equalRunes(folded[i:i+len(wr)], wr) {
				out = append(out, Match{Start: i, Length: len(wr)})
				found = true
				i += len(wr) - 1
			}
		}
		if !found {
			return nil
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out
}

func equalRunes(a, b []rune) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// makeSnippet cuts a window around the first match and returns it with the
// character offset where the window starts. Whitespace runs are collapsed so a
// snippet is one tidy line; offsets stay relative to the original text, so
// callers that highlight should use Matches against the full text instead.
func makeSnippet(text string, matches []Match) (string, int) {
	runes := []rune(text)
	first := matches[0]
	from := first.Start - snippetRadius
	if from < 0 {
		from = 0
	}
	to := first.Start + first.Length + snippetRadius
	if to > len(runes) {
		to = len(runes)
	}
	snippet := string(runes[from:to])
	snippet = strings.Join(strings.Fields(snippet), " ")
	if from > 0 {
		snippet = "…" + snippet
	}
	if to < len(runes) {
		snippet += "…"
	}
	if !utf8.ValidString(snippet) {
		snippet = strings.ToValidUTF8(snippet, "")
	}
	return snippet, from
}

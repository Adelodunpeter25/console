// Trashed (soft-deleted) sessions stay invisible to reads and searches by
// default; include_deleted opts back in. Settled sessions, by contrast, are
// ordinary live sessions and are always visible.
package tests

import (
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/session"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

func TestSearchSkipsDeletedUnlessIncluded(t *testing.T) {
	s, h := searchSvc(t)
	_ = s.AppendMessages(h.ID, []types.AgentMessage{
		say("u1", "user", "the trellis wiring is wrong"),
	})
	if _, err := s.SoftDelete(h.ID); err != nil {
		t.Fatal(err)
	}

	// Explicit session_id scope errors on a trashed session, as before.
	if _, err := s.Search(session.SearchScope{SessionID: h.ID}, "trellis", session.SearchOptions{}); err == nil {
		t.Fatal("want an error when naming a trashed session explicitly")
	}

	// A cwd scope silently skips it.
	r, err := s.Search(session.SearchScope{Cwd: h.Cwd}, "trellis", session.SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Hits) != 0 {
		t.Fatalf("hits = %v, want none from a trashed session by default", hitIDs(r))
	}

	r, err = s.Search(session.SearchScope{SessionID: h.ID, IncludeDeleted: true}, "trellis", session.SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Hits) != 1 || !r.Hits[0].SessionDeleted {
		t.Fatalf("hits = %+v, want one hit with session_deleted set from the trashed session", r.Hits)
	}
}

func TestSearchScopeListIncludesDeleted(t *testing.T) {
	s, h := searchSvc(t)
	_ = s.AppendMessages(h.ID, []types.AgentMessage{
		say("u1", "user", "note about the lighthouse"),
	})
	if _, err := s.SoftDelete(h.ID); err != nil {
		t.Fatal(err)
	}

	// A cwd scope walks ListFiltered, not Header, so it needs the
	// flag on the list path too.
	r, err := s.Search(session.SearchScope{Cwd: h.Cwd, IncludeDeleted: true}, "lighthouse", session.SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Hits) != 1 || !r.Hits[0].SessionDeleted {
		t.Fatalf("hits = %+v, want the trashed session's hit with session_deleted set", r.Hits)
	}
}

func TestLoadRefusesDeletedUnlessIncluded(t *testing.T) {
	s, h := searchSvc(t)
	_ = s.AppendMessages(h.ID, []types.AgentMessage{
		say("u1", "user", "the trellis wiring is wrong"),
		say("a1", "assistant", "fixed the trellis wiring"),
	})
	if _, err := s.SoftDelete(h.ID); err != nil {
		t.Fatal(err)
	}

	if loaded, err := s.Load(h.ID, 0, 0); err != nil || loaded != nil {
		t.Fatalf("Load on a trashed session = %v, %v; want nil, nil", loaded, err)
	}
	loaded, err := s.LoadIncludingDeleted(h.ID, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if loaded == nil || len(loaded.Messages) != 2 {
		t.Fatalf("LoadIncludingDeleted = %+v, want both messages", loaded)
	}
	if loaded.Header.DeletedAt == nil {
		t.Fatalf("header = %+v, want deleted_at kept so the caller can tell it is trashed", loaded.Header)
	}
}

func TestSettledSessionsStayVisible(t *testing.T) {
	s, h := searchSvc(t)
	_ = s.AppendMessages(h.ID, []types.AgentMessage{
		say("u1", "user", "remember the kerfuffle"),
	})
	if _, err := s.Settle(h.ID); err != nil {
		t.Fatal(err)
	}

	loaded, err := s.Load(h.ID, 0, 0)
	if err != nil || loaded == nil {
		t.Fatalf("Load on a settled session = %v, %v; want it readable", loaded, err)
	}
	r, err := s.Search(session.SearchScope{SessionID: h.ID}, "kerfuffle", session.SearchOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Hits) != 1 || r.Hits[0].SessionDeleted {
		t.Fatalf("hits = %+v, want the settled session searched with no deleted mark", r.Hits)
	}
}

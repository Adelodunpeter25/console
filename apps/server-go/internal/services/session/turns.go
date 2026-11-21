// Turn assembly: the server groups stored LLM rows into the transcript
// structure clients render, so no client re-derives runs, wake splits or
// tool-call/result pairing.
package session

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"google.golang.org/protobuf/encoding/protojson"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// inlineResultLimit is the largest tool-result body kept inside a turn.
// Bigger bodies are dropped and fetched on expand (see ToolResult), so a
// first page of a tool-heavy chat stays small.
const inlineResultLimit = 4096

const defaultWakeLabel = "Background work completed, continuing agent"

var rowUnmarshal = protojson.UnmarshalOptions{DiscardUnknown: true}

// LoadedTurns is one page of whole turns, newest page first. NextCursor is
// the rowid of the oldest returned turn's first row: pass it as before to
// get the turns preceding this page.
type LoadedTurns struct {
	Header     types.SessionHeader
	Turns      []*consolev1.TranscriptTurn
	HasMore    bool
	NextCursor *int64
}

// LoadTurns returns up to limit whole turns older than the before rowid
// (limit <= 0 means every turn). A turn starts at a user row, real or
// synthetic wake, and is never split across pages. active marks the newest
// turn as still running.
func (s *Service) LoadTurns(sessionID string, limit int64, before int64, active bool) (*LoadedTurns, error) {
	header, conn, err := s.openForRead(sessionID, false)
	if err != nil || header == nil {
		return nil, err
	}
	lower, hasMore, err := turnLowerBound(conn, limit, before)
	if err != nil {
		return nil, err
	}
	rows, err := readTurnRows(conn, hasContentColumn(conn), lower, before, "")
	if err != nil {
		return nil, err
	}
	projectID := projectIDFrom(header.ProjectID)
	bodies := s.materialize(sessionID, projectID, conn, rows)
	msgs := make([]RowMessage, 0, len(rows))
	for i, r := range rows {
		if bodies[i] == "" {
			continue
		}
		var m consolev1.AgentMessage
		if err := rowUnmarshal.Unmarshal(withCreatedAt(bodies[i], r.createdAt, r.id), &m); err != nil {
			continue
		}
		msgs = append(msgs, RowMessage{Rowid: r.rowid, Msg: &m})
	}
	out := &LoadedTurns{Header: *header, Turns: AssembleTurns(msgs, active), HasMore: hasMore}
	if hasMore {
		cursor := lower
		out.NextCursor = &cursor
	}
	return out, nil
}

// turnLowerBound finds the rowid of the first row to read for a page of
// limit turns. User rows are indexed, so this walks the index instead of
// scanning messages.
func turnLowerBound(conn *sql.DB, limit, before int64) (lower int64, hasMore bool, err error) {
	if limit <= 0 {
		return 0, false, nil
	}
	query := "SELECT rowid FROM messages WHERE role = 'user'"
	var args []any
	if before > 0 {
		query += " AND rowid < ?"
		args = append(args, before)
	}
	query += " ORDER BY rowid DESC LIMIT ?"
	args = append(args, limit+1)
	rs, err := conn.Query(query, args...)
	if err != nil {
		return 0, false, err
	}
	defer rs.Close()
	var starts []int64
	for rs.Next() {
		var id int64
		if err := rs.Scan(&id); err != nil {
			return 0, false, err
		}
		starts = append(starts, id)
	}
	if err := rs.Err(); err != nil {
		return 0, false, err
	}
	if int64(len(starts)) > limit {
		return starts[limit-1], true, nil
	}
	return 0, false, nil
}

func readTurnRows(conn *sql.DB, hasContent bool, lower, before int64, id string) ([]storedRow, error) {
	contentSel := "''"
	if hasContent {
		contentSel = "content"
	}
	query := "SELECT " + contentSel + ", created_at, rowid, id, skeleton, split_state FROM messages"
	var conds []string
	var args []any
	if id != "" {
		conds = append(conds, "id = ?")
		args = append(args, id)
	}
	if lower > 0 {
		conds = append(conds, "rowid >= ?")
		args = append(args, lower)
	}
	if before > 0 {
		conds = append(conds, "rowid < ?")
		args = append(args, before)
	}
	if len(conds) > 0 {
		query += " WHERE " + strings.Join(conds, " AND ")
	}
	query += " ORDER BY rowid"
	rs, err := conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	// Closed before the caller reads parts: the session database has one
	// connection, so a second query while these rows are open would deadlock.
	defer rs.Close()
	var rows []storedRow
	for rs.Next() {
		var r storedRow
		if err := rs.Scan(&r.content, &r.createdAt, &r.rowid, &r.id, &r.skeleton, &r.state); err != nil {
			return nil, err
		}
		rows = append(rows, r)
	}
	return rows, rs.Err()
}

// ErrToolResultNotFound is returned when a message or call id does not exist.
var ErrToolResultNotFound = errors.New("tool result not found")

// ToolResult returns the full result of one tool call from the tool-result
// message that holds it.
func (s *Service) ToolResult(sessionID, messageID, callID string) (*consolev1.ToolResult, error) {
	header, conn, err := s.openForRead(sessionID, true)
	if err != nil {
		return nil, err
	}
	if header == nil {
		return nil, ErrToolResultNotFound
	}
	rows, err := readTurnRows(conn, hasContentColumn(conn), 0, 0, messageID)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		isErr := true
		rawContent, _ := json.Marshal("Tool execution was interrupted (server stopped or restarted).")
		return &consolev1.ToolResult{
			ToolCallId: callID,
			Content:    rawContent,
			IsError:    &isErr,
		}, nil
	}
	bodies := s.materialize(sessionID, projectIDFrom(header.ProjectID), conn, rows)
	if bodies[0] == "" {
		return nil, ErrToolResultNotFound
	}
	var m consolev1.AgentMessage
	if err := rowUnmarshal.Unmarshal(json.RawMessage(bodies[0]), &m); err != nil {
		return nil, err
	}
	for _, res := range m.GetToolResult().GetResults() {
		if res.GetToolCallId() == callID {
			return res, nil
		}
	}
	return nil, ErrToolResultNotFound
}

// RowMessage is a decoded stored row with its rowid.
type RowMessage struct {
	Rowid int64
	Msg   *consolev1.AgentMessage
}

// AssembleTurns groups rows (oldest first) into turns. Rows before the first
// user row belong to no turn and are dropped. With active set, the newest
// turn is reported as working.
func AssembleTurns(rows []RowMessage, active bool) []*consolev1.TranscriptTurn {
	var turns []*consolev1.TranscriptTurn
	var cur *consolev1.TranscriptTurn
	var last int64
	calls := map[string]*consolev1.RunToolCall{}

	finish := func() {
		if cur == nil {
			return
		}
		if run := cur.Run; run != nil && last > cur.CreatedAt {
			run.ElapsedMs = last - cur.CreatedAt
		}
		turns = append(turns, cur)
		cur = nil
		clear(calls)
	}

	for _, row := range rows {
		m := row.Msg
		at := m.GetCreatedAt()
		if u := m.GetUser(); u != nil {
			finish()
			cur = &consolev1.TranscriptTurn{Id: turnID(m, row.Rowid), CreatedAt: at}
			if u.GetSynthetic() {
				label := u.GetWakeLabel()
				if label == "" {
					label = defaultWakeLabel
				}
				cur.WakeLabel = &label
			} else {
				cur.UserMessage = m
			}
			cur.Run = &consolev1.RunActivity{RunId: cur.Id, StartedAt: at, Status: consolev1.RunStatus_RUN_STATUS_COMPLETED}
			last = at
			continue
		}
		if cur == nil {
			continue
		}
		last = at
		switch {
		case m.GetAssistant() != nil:
			asst := m.GetAssistant()
			if !hasToolCall(asst) {
				cur.Responses = append(cur.Responses, m)
				continue
			}
			for _, part := range asst.GetContent() {
				switch p := part.GetPart().(type) {
				case *consolev1.AssistantContentPart_Thinking:
					if strings.TrimSpace(p.Thinking.GetText()) != "" {
						cur.Run.Events = append(cur.Run.Events, textEvent(cur, true, p.Thinking.GetText()))
					}
				case *consolev1.AssistantContentPart_Text:
					if strings.TrimSpace(p.Text.GetText()) != "" {
						cur.Run.Events = append(cur.Run.Events, textEvent(cur, false, p.Text.GetText()))
					}
				case *consolev1.AssistantContentPart_ToolCall:
					tc := &consolev1.RunToolCall{Call: p.ToolCall}
					calls[p.ToolCall.GetId()] = tc
					cur.Run.Events = append(cur.Run.Events, &consolev1.RunActivityEvent{
						Id:    p.ToolCall.GetId(),
						Event: &consolev1.RunActivityEvent_ToolCall{ToolCall: tc},
					})
				}
			}
		case m.GetToolResult() != nil:
			for _, res := range m.GetToolResult().GetResults() {
				if tc := calls[res.GetToolCallId()]; tc != nil {
					tc.Result, tc.ResultSize = summarizeResult(res)
					tc.ResultMessageId = m.GetId()
				}
			}
		}
	}
	finish()

	var isWorkingTurn bool
	if active && len(turns) > 0 {
		run := turns[len(turns)-1].Run
		run.Status = consolev1.RunStatus_RUN_STATUS_WORKING
		run.ElapsedMs = 0
		isWorkingTurn = true
	}
	for i, t := range turns {
		if t.Run != nil && len(t.Run.Events) == 0 && t.Run.Status != consolev1.RunStatus_RUN_STATUS_WORKING {
			t.Run = nil
			continue
		}
		if t.Run != nil && (!isWorkingTurn || i < len(turns)-1) {
			for _, ev := range t.Run.Events {
				if tc := ev.GetToolCall(); tc != nil && tc.Result == nil {
					isErr := true
					rawContent, _ := json.Marshal("Tool execution was interrupted (server stopped or restarted).")
					toolName := tc.Call.GetName()
					tc.Result = &consolev1.ToolResult{
						ToolCallId: tc.Call.GetId(),
						ToolName:   &toolName,
						Content:    rawContent,
						IsError:    &isErr,
					}
					tc.ResultSize = uint32(len(rawContent))
				}
			}
		}
	}
	return turns
}

func turnID(m *consolev1.AgentMessage, rowid int64) string {
	if id := m.GetId(); id != "" {
		return id
	}
	return strconv.FormatInt(rowid, 10)
}

func hasToolCall(a *consolev1.AgentAssistantMessage) bool {
	for _, p := range a.GetContent() {
		if p.GetToolCall() != nil {
			return true
		}
	}
	return false
}

func textEvent(t *consolev1.TranscriptTurn, thinking bool, text string) *consolev1.RunActivityEvent {
	n := strconv.Itoa(len(t.Run.Events))
	if thinking {
		return &consolev1.RunActivityEvent{Id: t.GetId() + "-thinking-" + n, Event: &consolev1.RunActivityEvent_Thinking{Thinking: text}}
	}
	return &consolev1.RunActivityEvent{Id: t.GetId() + "-text-" + n, Event: &consolev1.RunActivityEvent_Text{Text: text}}
}

// summarizeResult keeps small bodies and artifacts, drops large bodies, and
// always reports the original size.
func summarizeResult(res *consolev1.ToolResult) (*consolev1.ToolResult, uint32) {
	size := uint32(len(res.GetContent()))
	if size <= inlineResultLimit {
		return res, size
	}
	return &consolev1.ToolResult{
		ToolCallId: res.GetToolCallId(),
		ToolName:   res.ToolName,
		IsError:    res.IsError,
		Visual:     res.Visual,
		Html:       res.Html,
	}, size
}

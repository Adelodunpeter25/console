// Per-session built-in capabilities (computer-use, visualize, ...): which are
// switched on survives a server restart because it lives in the session row.
package session

import "strings"

// Capabilities returns the capabilities switched on for a session.
func (s *Service) Capabilities(sessionID string) ([]string, error) {
	var raw *string
	if err := s.manager.Global().QueryRow(
		`SELECT capabilities FROM sessions WHERE id = ?`, sessionID).Scan(&raw); err != nil {
		return nil, nil // unknown session: nothing is on
	}
	if raw == nil || *raw == "" {
		return nil, nil
	}
	return strings.Split(*raw, ","), nil
}

// AddCapability switches a capability on for a session. Idempotent, and it
// does not touch updated_at: activating one must not reorder the sidebar.
func (s *Service) AddCapability(sessionID, name string) error {
	current, _ := s.Capabilities(sessionID)
	for _, existing := range current {
		if existing == name {
			return nil
		}
	}
	_, err := s.manager.Global().Exec(
		`UPDATE sessions SET capabilities = ? WHERE id = ?`,
		strings.Join(append(current, name), ","), sessionID)
	return err
}

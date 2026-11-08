// console.automation.* tools: the agent schedules its own future work on the
// current session. One active automation per session; completion is
// prompt-driven — the agent deletes the automation when it observes done.
package tools

import (
	"context"
	"fmt"
	"time"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/automations"
)

// AutomationStore is the persistence seam, satisfied by *automations.Service.
type AutomationStore interface {
	Create(sessionID, prompt, kind string, intervalSeconds, runAt int64, now time.Time) (*automations.Automation, error)
	Get(id string) (*automations.Automation, error)
	ActiveForSession(sessionID string) (*automations.Automation, error)
	ListForSession(sessionID string) ([]automations.Automation, error)
	Update(id, prompt, kind string, intervalSeconds, runAt int64, now time.Time) (*automations.Automation, error)
	SetStatus(id, status string) (bool, error)
	Delete(id string) (bool, error)
}

func automationUnavailable() (any, error) {
	return nil, NewToolError("Automations are unavailable on this server")
}

func renderAutomation(a *automations.Automation) string {
	out := fmt.Sprintf("id: %s\nstatus: %s\nkind: %s\nprompt: %s", a.ID, a.Status, a.Kind, a.Prompt)
	if a.Kind == automations.KindInterval {
		out += fmt.Sprintf("\ninterval: %ds", a.IntervalSeconds)
	} else {
		out += fmt.Sprintf("\nrunAt: %s", time.Unix(a.RunAt, 0).Format(time.RFC3339))
	}
	out += fmt.Sprintf("\nnextRunAt: %s", time.Unix(a.NextRunAt, 0).Format(time.RFC3339))
	if a.LastError != "" {
		out += "\nlastError: " + a.LastError
	}
	return out
}

// NewAutomationCreateTool schedules future work on the session that calls it.
func NewAutomationCreateTool(sessionID string, store AutomationStore) Tool {
	type input struct {
		Prompt string `json:"prompt" jsonschema:"required,description=The instruction to run when the schedule fires. It runs in this same thread with full context. Say what to inspect or change\\, what to report\\, and when to stop"`
		Kind   string `json:"kind" jsonschema:"required,description='once' for a single future run\\, 'interval' for a repeating run"`
		// intervalSeconds and runAt stay int64 so a misparsed timing fails
		// validation instead of silently firing at epoch.
		IntervalSeconds int64 `json:"intervalSeconds,omitempty" jsonschema:"description=For kind 'interval': seconds between fires (e.g. 300 for every 5 minutes)"`
		RunAt           int64 `json:"runAt,omitempty" jsonschema:"description=For kind 'once': absolute unix timestamp (seconds) to fire at"`
	}
	run := func(ctx context.Context, in input) (any, error) {
		if store == nil {
			return automationUnavailable()
		}
		existing, err := store.ActiveForSession(sessionID)
		if err != nil {
			return nil, NewToolError("Failed to check existing automation: %v", err)
		}
		if existing != nil {
			return nil, NewToolError("This session already has an active automation (%s): %s. Update or delete it first.", existing.ID, existing.Prompt)
		}
		a, err := store.Create(sessionID, in.Prompt, in.Kind, in.IntervalSeconds, in.RunAt, time.Now())
		if err != nil {
			return nil, NewToolError("Failed to create automation: %v", err)
		}
		return map[string]any{"content": "Automation created.\n" + renderAutomation(a), "id": a.ID}, nil
	}
	return NewTool("console.automation.create",
		"Schedule future work on this thread. 'once' fires at a unix timestamp (e.g. a reminder in two days); 'interval' repeats every intervalSeconds seconds (e.g. 300 = every 5 minutes). The prompt runs in this same thread with full context. Prefer one automation per monitoring task: when it is done, delete it with console.automation.delete.",
		TierWrite, run)
}

func NewAutomationGetTool(sessionID string, store AutomationStore) Tool {
	type input struct {
		ID string `json:"id" jsonschema:"description=Automation id. Omit to list this session's automations"`
	}
	run := func(ctx context.Context, in input) (any, error) {
		if store == nil {
			return automationUnavailable()
		}
		if in.ID == "" {
			list, err := store.ListForSession(sessionID)
			if err != nil {
				return nil, NewToolError("Failed to list automations: %v", err)
			}
			if len(list) == 0 {
				return "No automations on this session.", nil
			}
			out := ""
			for i := range list {
				out += renderAutomation(&list[i]) + "\n\n"
			}
			return out, nil
		}
		a, err := store.Get(in.ID)
		if err != nil {
			return nil, NewToolError("Failed to load automation: %v", err)
		}
		if a == nil || a.SessionID != sessionID {
			return nil, NewToolError("No automation %s on this session", in.ID)
		}
		return renderAutomation(a), nil
	}
	return NewTool("console.automation.get",
		"Get one automation by id, or list every automation on this session.", TierRead, run)
}

func NewAutomationUpdateTool(sessionID string, store AutomationStore) Tool {
	type input struct {
		ID              string `json:"id" jsonschema:"required,description=Automation id to update"`
		Prompt          string `json:"prompt,omitempty" jsonschema:"description=New prompt"`
		Kind            string `json:"kind,omitempty" jsonschema:"description='once' or 'interval'"`
		IntervalSeconds int64  `json:"intervalSeconds,omitempty" jsonschema:"description=For kind 'interval': seconds between fires"`
		RunAt           int64  `json:"runAt,omitempty" jsonschema:"description=For kind 'once': absolute unix timestamp (seconds)"`
	}
	run := func(ctx context.Context, in input) (any, error) {
		if store == nil {
			return automationUnavailable()
		}
		existing, err := store.Get(in.ID)
		if err != nil {
			return nil, NewToolError("Failed to load automation: %v", err)
		}
		if existing == nil || existing.SessionID != sessionID {
			return nil, NewToolError("No automation %s on this session", in.ID)
		}
		prompt, kind := existing.Prompt, existing.Kind
		if in.Prompt != "" {
			prompt = in.Prompt
		}
		if in.Kind != "" {
			kind = in.Kind
		}
		interval, runAt := existing.IntervalSeconds, existing.RunAt
		if in.IntervalSeconds > 0 {
			interval = in.IntervalSeconds
		}
		if in.RunAt > 0 {
			runAt = in.RunAt
		}
		a, err := store.Update(in.ID, prompt, kind, interval, runAt, time.Now())
		if err != nil {
			return nil, NewToolError("Failed to update automation: %v", err)
		}
		if a == nil {
			return nil, NewToolError("Automation %s is %s and cannot be updated", in.ID, existing.Status)
		}
		return "Automation updated.\n" + renderAutomation(a), nil
	}
	return NewTool("console.automation.update",
		"Change an automation's prompt, timing, or kind. Also re-arms its next run from now.", TierWrite, run)
}

func NewAutomationDeleteTool(sessionID string, store AutomationStore) Tool {
	type input struct {
		ID string `json:"id" jsonschema:"required,description=Automation id to delete"`
	}
	run := func(ctx context.Context, in input) (any, error) {
		if store == nil {
			return automationUnavailable()
		}
		existing, err := store.Get(in.ID)
		if err != nil {
			return nil, NewToolError("Failed to load automation: %v", err)
		}
		if existing == nil || existing.SessionID != sessionID {
			return nil, NewToolError("No automation %s on this session", in.ID)
		}
		if _, err := store.Delete(in.ID); err != nil {
			return nil, NewToolError("Failed to delete automation: %v", err)
		}
		return fmt.Sprintf("Automation %s deleted.", in.ID), nil
	}
	return NewTool("console.automation.delete",
		"Delete an automation. Call this when a monitoring task is complete — the harness does not know when your task is done.", TierWrite, run)
}

// ask / askMany tools: interactive questions to the user. Port of
// apps/server/agent/src/tools/ask.ts. Both take an optional AskHandler;
// without one (headless), they auto-select the first option (or report
// "skipped") so the tools still work when nothing is attached to answer.
package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/utils"
)

// AskQuestionRequest is emitted to the attached handler (and, in the real
// server, broadcast to the client) when the model asks a question.
type AskQuestionRequest struct {
	RequestID     string   `json:"requestId"`
	Question      string   `json:"question"`
	Options       []string `json:"options,omitempty"`
	IsMultiSelect bool     `json:"isMultiSelect,omitempty"`
	Skippable     bool     `json:"skippable"`
	BatchID       string   `json:"batchId,omitempty"`
}

// AskAnswer is the user's reply: Multi is set for multi-select questions,
// Text otherwise (free-text or single-select). An empty Text with no Multi
// entries means the question was skipped.
type AskAnswer struct {
	Text  string
	Multi []string
}

func (a AskAnswer) format() string {
	var text string
	if len(a.Multi) > 0 {
		text = strings.Join(a.Multi, ", ")
	} else {
		text = a.Text
	}
	if text == "" {
		return "[User Answer]: User skipped this question."
	}
	return fmt.Sprintf("[User Answer]: %q", text)
}

// AskHandler answers one question, blocking until the user (or caller)
// responds. ctx cancellation aborts the wait.
type AskHandler func(ctx context.Context, req AskQuestionRequest) (AskAnswer, error)

type askQuestionInput struct {
	Question      string   `json:"question" jsonschema:"required,description=The question to ask the user"`
	Options       []string `json:"options,omitempty" jsonschema:"description=Optional list of selectable multiple-choice options for the user. Omit for a free-text question."`
	IsMultiSelect bool     `json:"isMultiSelect,omitempty" jsonschema:"description=Set to true if user can select multiple options"`
	Skippable     *bool    `json:"skippable,omitempty" jsonschema:"description=Set to false if this question is REQUIRED and cannot be skipped by the user. Defaults to true (optional)."`
}

func (in askQuestionInput) skippable() bool {
	return in.Skippable == nil || *in.Skippable
}

func askOne(ctx context.Context, handler AskHandler, in askQuestionInput, batchID string) (AskAnswer, error) {
	req := AskQuestionRequest{
		RequestID:     utils.RandomID(),
		Question:      in.Question,
		Options:       in.Options,
		IsMultiSelect: in.IsMultiSelect,
		Skippable:     in.skippable(),
		BatchID:       batchID,
	}
	return handler(ctx, req)
}

func headlessAnswer(question string, options []string) string {
	defaultAnswer := "skipped"
	if len(options) > 0 {
		defaultAnswer = options[0]
	}
	return fmt.Sprintf("[Asked User]: %q\nSelected default option: %q", question, defaultAnswer)
}

// NewAskTool builds the "ask" tool bound to handler. A nil handler falls
// back to a headless default-option response (used for the DefaultTools()
// singleton and any run without an interactive channel attached).
func NewAskTool(handler AskHandler) Tool {
	return NewTool("ask", "Ask the user a question to clarify requirements. Optionally provide distinct choices.", TierRead,
		func(ctx context.Context, in askQuestionInput) (any, error) {
			if in.Question == "" {
				return nil, NewToolError("question is required")
			}
			if handler == nil {
				return headlessAnswer(in.Question, in.Options), nil
			}
			answer, err := askOne(ctx, handler, in, "")
			if err != nil {
				return nil, err
			}
			return answer.format(), nil
		})
}

type askManyInput struct {
	Questions []askQuestionInput `json:"questions" jsonschema:"required,description=The list of questions to ask the user, answered in order"`
}

// NewAskManyTool builds the "askMany" tool bound to handler, asking each
// question in sequence and sharing one batch id across the group.
func NewAskManyTool(handler AskHandler) Tool {
	return NewTool("askMany", "Ask the user multiple questions in sequence.", TierRead,
		func(ctx context.Context, in askManyInput) (any, error) {
			if len(in.Questions) == 0 {
				return nil, NewToolError("questions must contain at least one entry")
			}
			if handler == nil {
				lines := make([]string, 0, len(in.Questions))
				for _, q := range in.Questions {
					lines = append(lines, headlessAnswer(q.Question, q.Options))
				}
				return strings.Join(lines, "\n\n"), nil
			}
			batchID := utils.RandomID()
			lines := make([]string, 0, len(in.Questions))
			for i, q := range in.Questions {
				answer, err := askOne(ctx, handler, q, batchID)
				if err != nil {
					return nil, err
				}
				lines = append(lines, fmt.Sprintf("Q%d: %s", i+1, answer.format()))
			}
			return strings.Join(lines, "\n"), nil
		})
}

// Ask and AskMany are the headless default-answer instances used by
// DefaultTools(); a run with an interactive channel builds its own via
// NewAskTool/NewAskManyTool with a real handler instead.
var Ask = NewAskTool(nil)
var AskMany = NewAskManyTool(nil)

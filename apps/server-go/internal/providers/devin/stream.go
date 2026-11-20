// Devin Cascade streaming provider: implements loop.Provider by driving
// GetUserJwt → (AssignModel) → GetChatMessage per turn and translating the
// Connect stream into loop events. Ports oh-my-pi's streamDevin.
package devin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/accounts"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/loop"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/stream"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/agent/tools"
	exav1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/exa/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/providers/shared"
)

// Provider streams Devin Cascade turns. BaseURL, HTTPClient, and
// CredentialLoader are overridable for tests.
type Provider struct {
	BaseURL          string
	HTTPClient       *http.Client
	CredentialLoader func() (ParsedCredential, error)

	assignments assignmentCache
}

func (p *Provider) baseURL() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	return BaseURL
}

func (p *Provider) httpClient() *http.Client {
	if p.HTTPClient != nil {
		return p.HTTPClient
	}
	return shared.Client
}

func (p *Provider) loadCredential(accountID string) (ParsedCredential, error) {
	if p.CredentialLoader != nil {
		return p.CredentialLoader()
	}
	if accountID != "" {
		cred, err := LoadCredentialFor(accountID)
		if err != nil {
			return ParsedCredential{}, accounts.PinnedError("devin", accountID, err)
		}
		return cred, nil
	}
	return LoadCredential()
}

// devinDefaultStopPatterns mirrors DEVIN_DEFAULT_STOP_PATTERNS upstream.
var devinDefaultStopPatterns = []string{
	"<|user|>", "<|bot|>", "<|context_request|>", "", "<|end_of_turn|>",
}

// streamedToolCall accumulates one tool call's argumentsJson, which arrives
// cumulative (each deltaToolCalls entry repeats the full JSON so far).
type streamedToolCall struct {
	name      string
	arguments string
}

// RunTurn implements loop.Provider: one streaming assistant turn.
func (p *Provider) RunTurn(ctx context.Context, req loop.TurnRequest, events *stream.Stream[loop.Event]) error {
	err := p.runTurn(ctx, req, events)
	if err != nil {
		// Early failures (credentials, auth, model assignment) return before
		// the stream is closed; without this the consumer blocks forever and
		// the run never releases its session slot. Fail is a no-op when the
		// stream is already done.
		events.Fail(err)
	}
	return err
}

func (p *Provider) runTurn(ctx context.Context, req loop.TurnRequest, events *stream.Stream[loop.Event]) error {
	cred, err := p.loadCredential(req.AccountID)
	if err != nil {
		return err
	}
	cascadeID := req.ConversationID
	if cascadeID == "" {
		cascadeID = uuid.NewString()
	}

	prompts := buildPrompts(loop.WithTail(req.Tail, req.Messages), cascadeID)
	routerPrompt := lastUserPrompt(prompts)

	chatUID, meta := chatModelUID(req.Model, req.ThinkingLevel)
	toolDefs := buildToolDefinitions(req.Tools, isGeminiModelUID(chatUID))

	auth, err := fetchTurnAuth(ctx, p.httpClient(), p.baseURL(), cred.Token, cascadeID)
	if err != nil {
		return err
	}

	// Router models ("adaptive") are not valid chat uids: AssignModel
	// resolves them into a concrete uid + assignment JWT on the shared
	// cascade id.
	var assignment *modelAssignment
	if meta.isAssignRouter {
		routerUID := chatUID
		if pinned, ok := p.assignments.get(cascadeID, routerUID, time.Now()); ok {
			assignment = pinned
		} else {
			assignment, err = assignModel(ctx, p.httpClient(), auth, chatUID, routerPrompt)
			if err != nil {
				return err
			}
			p.assignments.put(cascadeID, routerUID, assignment, time.Now())
		}
		chatUID = assignment.modelUID
	}

	maxTokens := meta.maxOutputTokens
	if maxTokens <= 0 {
		maxTokens = defaultMaxTokens
	}
	request := &exav1.GetChatMessageRequest{
		Metadata:           cliSessionMetadata(auth.apiKey, auth.userJWT),
		Prompt:             req.SystemPrompt,
		ChatMessagePrompts: prompts,
		ChatModelUid:       chatUID,
		RequestType:        exav1.ChatMessageRequestType_CHAT_MESSAGE_REQUEST_TYPE_CASCADE,
		Configuration: &exav1.CompletionConfiguration{
			NumCompletions:      1,
			MaxTokens:           uint64(maxTokens),
			MaxNewlines:         200,
			Temperature:         0.4,
			FirstTemperature:    0.4,
			TopK:                50,
			TopP:                1,
			StopPatterns:        devinDefaultStopPatterns,
			FimEotProbThreshold: 1,
		},
		Tools:                    toolDefs,
		DisableParallelToolCalls: !meta.parallelTools,
		ToolChoice:               &exav1.ChatToolChoice{Choice: &exav1.ChatToolChoice_OptionName{OptionName: "auto"}},
		SystemPromptCacheOptions: &exav1.PromptCacheOptions{Type: exav1.CacheControlType_CACHE_CONTROL_TYPE_EPHEMERAL},
		CascadeId:                cascadeID,
		PlannerMode:              exav1.ConversationalPlannerMode_CONVERSATIONAL_PLANNER_MODE_DEFAULT,
		ExecutionId:              uuid.NewString(),
	}
	if assignment != nil {
		request.ModelAssignmentJwt = assignment.jwt
	}

	// Accumulate streamed state. signature is the latest thinking
	// signature (Devin sends deltaSignature alongside deltaThinking); it
	// rides EventThinking.ThoughtSignature into ThinkingPart.Signature.
	toolCalls := map[string]*streamedToolCall{}
	var toolOrder []string
	var activeToolCallID string
	var lastUsage *exav1.ModelUsageStats
	var signature string

	historyBytes := estimateHistoryBytes(prompts)

	err = streamChat(ctx, p.httpClient(), auth.baseURL, chatPath, request, func(msg *exav1.GetChatMessageResponse) error {
		if msg.GetDeltaSignature() != "" {
			signature = msg.GetDeltaSignature()
		}
		if msg.GetDeltaThinking() != "" {
			events.Push(loop.Event{Kind: loop.EventThinking, Text: msg.GetDeltaThinking(), ThoughtSignature: signature})
		}
		if msg.GetDeltaText() != "" {
			events.Push(loop.Event{Kind: loop.EventText, Text: msg.GetDeltaText()})
		}
		for _, tc := range msg.GetDeltaToolCalls() {
			id := tc.GetId()
			if id == "" {
				id = activeToolCallID
			}
			if id == "" {
				continue
			}
			block, ok := toolCalls[id]
			if !ok {
				block = &streamedToolCall{}
				toolCalls[id] = block
				toolOrder = append(toolOrder, id)
			}
			if tc.GetName() != "" {
				block.name = tc.GetName()
			}
			activeToolCallID = id
			if tc.GetArgumentsJson() != "" {
				// argumentsJson is cumulative: each entry repeats the full
				// JSON so far, so keep the longest seen.
				if len(tc.GetArgumentsJson()) > len(block.arguments) {
					block.arguments = tc.GetArgumentsJson()
				}
			}
		}
		if msg.GetUsage() != nil {
			lastUsage = msg.GetUsage()
		}
		return nil
	})

	// Emit finalized tool calls before usage/termination (Push after
	// termination drops). A cancelled stream leaves cumulative arguments
	// cut off mid-JSON, so partial calls are dropped.
	if ctx.Err() != nil {
		toolOrder = nil
	}
	for _, id := range toolOrder {
		block := toolCalls[id]
		args := block.arguments
		if args == "" {
			args = "{}"
		}
		events.Push(loop.Event{
			Kind: loop.EventToolCall,
			Call: &tools.ToolCall{ID: id, Name: block.name, Arguments: json.RawMessage(args)},
		})
	}

	usage := normalizeUsage(lastUsage)
	if usage == nil {
		usage = &loop.TurnUsage{CacheStatus: loop.CacheUnknown}
	}
	events.Push(loop.Event{Kind: loop.EventUsage, Usage: usage})

	if err != nil {
		if isContextOverflow(err, historyBytes) {
			// Shape the message to match compaction.IsContextOverflowError
			// so the loop's emergency compaction retries with shrunk history.
			err = fmt.Errorf("Devin context window exceeded: %w", err)
		}
		events.Fail(err)
		return err
	}
	events.Complete()
	return nil
}

// lastUserPrompt returns the trailing USER-source prompt for AssignModel's
// routing score, or nil. Native leaves messageId empty here — the id for
// the turn is minted by the chat request that follows.
func lastUserPrompt(prompts []*exav1.ChatMessagePrompt) *exav1.ChatMessagePrompt {
	for i := len(prompts) - 1; i >= 0; i-- {
		if prompts[i].GetSource() == exav1.ChatMessageSource_CHAT_MESSAGE_SOURCE_USER {
			cp := proto.Clone(prompts[i]).(*exav1.ChatMessagePrompt)
			cp.MessageId = ""
			return cp
		}
	}
	return nil
}

// estimateHistoryBytes approximates the serialized chatMessagePrompts size
// for the context-overflow recovery check (oh-my-pi re-encodes the field).
func estimateHistoryBytes(prompts []*exav1.ChatMessagePrompt) int {
	total := 0
	for _, p := range prompts {
		total += len(p.GetPrompt()) + len(p.GetThinking()) + len(p.GetSignature())
		for _, tc := range p.GetToolCalls() {
			total += len(tc.GetArgumentsJson()) + len(tc.GetName())
		}
		for _, img := range p.GetImages() {
			total += len(img.GetBase64Data())
		}
	}
	return total
}

// normalizeUsage maps ModelUsageStats onto loop.TurnUsage.
func normalizeUsage(u *exav1.ModelUsageStats) *loop.TurnUsage {
	if u == nil {
		return nil
	}
	input := int(u.GetInputTokens())
	cacheRead := int(u.GetCacheReadTokens())
	cacheWrite := int(u.GetCacheWriteTokens())
	output := int(u.GetOutputTokens())
	return &loop.TurnUsage{
		Input:       input,
		CacheRead:   cacheRead,
		CacheWrite:  cacheWrite,
		Output:      output,
		TotalTokens: input + cacheRead + cacheWrite + output,
		CacheStatus: loop.CacheHit,
	}
}

// isContextOverflow reports the Devin-specific overflow signature: an
// invalid_argument "internal error" trailer with a large history is the
// server rejecting an oversized request (oh-my-pi #4218 recovery).
func isContextOverflow(err error, historyBytes int) bool {
	if historyBytes < largeHistoryRecoveryBytes {
		return false
	}
	var streamErr *ConnectStreamError
	if !errors.As(err, &streamErr) {
		return false
	}
	if !strings.EqualFold(streamErr.Code, "invalid_argument") {
		return false
	}
	return strings.Contains(strings.ToLower(streamErr.Message), "internal error")
}
// The console.pr.* methods: the agent surface over the same pull-request
// service the HTTP routes serve. The kernel is a transport, not a bypass —
// reads and writes inherit the console tool's own exec-tier approval, and a
// restricted run cannot merge by creating a more permissive session (the
// ceiling applies at every call). Results carry the server-computed
// merge_state / merge_blocker so the model is told why a merge cannot
// proceed rather than retrying. The token never appears in a result.
package console

import (
	"context"
	"encoding/json"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	consolev1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/console/v1"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services"
	"github.com/Adelodunpeter25/console/apps/server-go/internal/services/jskernel"
)

// SetPullRequests attaches the pull-request service the pr.* methods serve.
func (r *Runtime) SetPullRequests(pulls *services.PullRequestService) {
	r.pulls = pulls
}

// prService returns the attached service or a clear error.
func (r *Runtime) prService() (*services.PullRequestService, error) {
	if r.pulls == nil {
		return nil, fmt.Errorf("pull-request service is unavailable")
	}
	return r.pulls, nil
}

// prRepo resolves the target repository: explicit owner/repo in the args,
// otherwise the session's project (its origin remote) — so the common call
// inside a project takes no repo arguments at all.
func (r *Runtime) prRepo(args map[string]any) (string, string, error) {
	owner, _ := args["owner"].(string)
	repo, _ := args["repo"].(string)
	if owner != "" && repo != "" {
		return owner, repo, nil
	}
	if r.projectID == "" {
		return "", "", fmt.Errorf("owner and repo are required (or run inside a project, so they resolve from its origin remote)")
	}
	pulls, err := r.prService()
	if err != nil {
		return "", "", err
	}
	return pulls.ResolveRepo(r.projectID)
}

// prNumberArg reads a whole-number PR number. The kernel hands small
// integers through as int; be liberal in what the number may arrive as.
func prNumberArg(args map[string]any) (uint32, error) {
	var value float64
	switch n := args["number"].(type) {
	case float64:
		value = n
	case int:
		value = float64(n)
	case int32:
		value = float64(n)
	case int64:
		value = float64(n)
	default:
		return 0, fmt.Errorf("number must be a whole pull-request number")
	}
	if value != float64(uint32(value)) || uint32(value) == 0 {
		return 0, fmt.Errorf("number must be a whole pull-request number")
	}
	return uint32(value), nil
}

// marshalProtoResult renders a proto message through protojson (canonical
// snake/low-camel field names the clients decode) as the call's result.
func marshalProtoResult(msg proto.Message) (*jskernel.Result, error) {
	raw, err := protojson.Marshal(msg)
	if err != nil {
		return nil, err
	}
	return marshalResult(json.RawMessage(raw))
}

func (r *Runtime) prList(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	pulls, err := r.prService()
	if err != nil {
		return nil, err
	}
	scope, _ := args["scope"].(string)
	if scope == "" {
		scope = "mine"
	}
	state, _ := args["state"].(string)
	if state == "" {
		state = "open"
	}
	draft, _ := args["draft"].(string)
	projectID, _ := args["project_id"].(string)
	if scope == "repo" && projectID == "" {
		projectID = r.projectID
	}
	list, err := pulls.List(ctx, scope, state, projectID, draft)
	if err != nil {
		return nil, err
	}
	return marshalProtoResult(list)
}

func (r *Runtime) prRead(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	pulls, err := r.prService()
	if err != nil {
		return nil, err
	}
	owner, repo, err := r.prRepo(args)
	if err != nil {
		return nil, err
	}
	number, err := prNumberArg(args)
	if err != nil {
		return nil, err
	}
	detail, err := pulls.Detail(ctx, owner, repo, number)
	if err != nil {
		return nil, err
	}
	return marshalProtoResult(detail)
}

func (r *Runtime) prCreate(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	pulls, err := r.prService()
	if err != nil {
		return nil, err
	}
	req := &consolev1.CreatePullRequestRequest{}
	req.Owner, req.Repo, err = r.prRepo(args)
	if err != nil {
		return nil, err
	}
	if value, ok := args["project_id"].(string); ok {
		req.ProjectId = value
	}
	req.Title, _ = args["title"].(string)
	req.Body, _ = args["body"].(string)
	req.Head, _ = args["head"].(string)
	req.Base, _ = args["base"].(string)
	if value, ok := args["draft"].(bool); ok {
		req.Draft = &value
	}
	// draft stays unset otherwise, which the service reads as true: an
	// agent-created PR is a draft until a human marks it ready.
	resp, err := pulls.Create(ctx, req)
	if err != nil {
		return nil, err
	}
	return marshalProtoResult(resp)
}

func (r *Runtime) prUpdate(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	pulls, err := r.prService()
	if err != nil {
		return nil, err
	}
	req := &consolev1.UpdatePullRequestRequest{}
	req.Owner, req.Repo, err = r.prRepo(args)
	if err != nil {
		return nil, err
	}
	if req.Number, err = prNumberArg(args); err != nil {
		return nil, err
	}
	if value, ok := args["title"].(string); ok {
		req.Title = &value
	}
	if value, ok := args["body"].(string); ok {
		req.Body = &value
	}
	if value, ok := args["base"].(string); ok {
		req.Base = &value
	}
	if value, ok := args["state"].(string); ok {
		req.State = value
	}
	resp, err := pulls.Update(ctx, req)
	if err != nil {
		return nil, err
	}
	return marshalProtoResult(resp)
}

func (r *Runtime) prComment(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	pulls, err := r.prService()
	if err != nil {
		return nil, err
	}
	req := &consolev1.CreateCommentRequest{}
	req.Owner, req.Repo, err = r.prRepo(args)
	if err != nil {
		return nil, err
	}
	if req.Number, err = prNumberArg(args); err != nil {
		return nil, err
	}
	req.Body, _ = args["body"].(string)
	resp, err := pulls.Comment(ctx, req)
	if err != nil {
		return nil, err
	}
	return marshalProtoResult(resp)
}

// prMerge merges. An omitted expected_head is filled from a fresh read, so
// the call merges only the exact commit the agent last saw; the result
// reports the SHA GitHub actually merged.
func (r *Runtime) prMerge(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	pulls, err := r.prService()
	if err != nil {
		return nil, err
	}
	req := &consolev1.MergePullRequestRequest{}
	req.Owner, req.Repo, err = r.prRepo(args)
	if err != nil {
		return nil, err
	}
	if req.Number, err = prNumberArg(args); err != nil {
		return nil, err
	}
	req.Method, _ = args["method"].(string)
	if req.ExpectedHead, _ = args["expected_head"].(string); req.ExpectedHead == "" {
		detail, err := pulls.Detail(ctx, req.Owner, req.Repo, req.Number)
		if err != nil {
			return nil, err
		}
		req.ExpectedHead = detail.HeadRefOid
	}
	resp, err := pulls.Merge(ctx, req)
	if err != nil {
		return nil, err
	}
	return marshalProtoResult(resp)
}

// prClose, prReopen and prReady are the one-field update conveniences.
func (r *Runtime) prClose(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	return r.prStateUpdate(ctx, args, func(req *consolev1.UpdatePullRequestRequest) { req.State = "closed" })
}

func (r *Runtime) prReopen(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	return r.prStateUpdate(ctx, args, func(req *consolev1.UpdatePullRequestRequest) { req.State = "open" })
}

// prReady flips a draft to ready for review.
func (r *Runtime) prReady(ctx context.Context, args map[string]any) (*jskernel.Result, error) {
	return r.prStateUpdate(ctx, args, func(req *consolev1.UpdatePullRequestRequest) {
		ready := false
		req.Draft = &ready
	})
}

func (r *Runtime) prStateUpdate(ctx context.Context, args map[string]any, apply func(*consolev1.UpdatePullRequestRequest)) (*jskernel.Result, error) {
	pulls, err := r.prService()
	if err != nil {
		return nil, err
	}
	req := &consolev1.UpdatePullRequestRequest{}
	if req.Owner, req.Repo, err = r.prRepo(args); err != nil {
		return nil, err
	}
	if req.Number, err = prNumberArg(args); err != nil {
		return nil, err
	}
	apply(req)
	resp, err := pulls.Update(ctx, req)
	if err != nil {
		return nil, err
	}
	return marshalProtoResult(resp)
}

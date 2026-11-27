// Per-turn Devin auth: GetUserJwt mints the user JWT (and can return a
// customApiServerUrl for enterprise seats); AssignModel resolves router
// uids ("adaptive" and friends) into a concrete model uid + assignment JWT.
package devin

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	exav1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/exa/v1"
)

// turnAuth is the per-turn wire state shared by AssignModel and
// GetChatMessage. cascadeID must agree across both calls or the JWT is
// rejected.
type turnAuth struct {
	apiKey    string // wire-encoded session token actually sent
	userJWT   string
	baseURL   string // customApiServerUrl override, when the seat returned one
	cascadeID string
}

// requestUserJWT calls AuthService/GetUserJwt. The session-token-prefixed
// apiKey is tried first; a 401 retries with the raw key (legacy Windsurf
// seats reject the prefix).
func requestUserJWT(ctx context.Context, client *http.Client, baseURL, sessionToken string) (userJWT, wireAPIKey, customBaseURL string, err error) {
	attempt := func(apiKey string) (*exav1.GetUserJwtResponse, int, error) {
		req := &exav1.GetUserJwtRequest{Metadata: cliSessionMetadata(apiKey, "")}
		resp := &exav1.GetUserJwtResponse{}
		// postUnary collapses status into the error; for the 401 retry we
		// need the status, so do the request inline via a status-capturing
		// variant.
		status, uerr := postUnaryStatus(ctx, client, baseURL, authPath, req, resp)
		return resp, status, uerr
	}

	prefixed := normalizeSessionToken(sessionToken)
	resp, status, uerr := attempt(prefixed)
	wireKey := prefixed
	if status == http.StatusUnauthorized && prefixed != sessionToken {
		resp, status, uerr = attempt(sessionToken)
		wireKey = sessionToken
	}
	if uerr != nil {
		return "", "", "", uerr
	}
	if resp.GetUserJwt() == "" {
		return "", "", "", fmt.Errorf("Devin auth error: GetUserJwt returned an empty user JWT")
	}
	custom := strings.TrimRight(strings.TrimSpace(resp.GetCustomApiServerUrl()), "/")
	return resp.GetUserJwt(), wireKey, custom, nil
}

// fetchTurnAuth resolves the session token into per-turn auth state.
func fetchTurnAuth(ctx context.Context, client *http.Client, baseURL, sessionToken, cascadeID string) (*turnAuth, error) {
	userJWT, wireKey, custom, err := requestUserJWT(ctx, client, baseURL, sessionToken)
	if err != nil {
		return nil, err
	}
	effective := baseURL
	if custom != "" {
		effective = custom
	}
	return &turnAuth{
		apiKey:    wireKey,
		userJWT:   userJWT,
		baseURL:   effective,
		cascadeID: cascadeID,
	}, nil
}

// modelAssignment is the AssignModel result: the concrete uid plus the JWT
// authorizing it on the shared cascade id.
type modelAssignment struct {
	jwt      string
	modelUID string
}

// assignModel resolves a server-side router uid ("adaptive") into the
// concrete model uid plus assignment JWT. The router uid is never a legal
// chatModelUid, so a failed assignment must fail the turn rather than fall
// back to sending the router id to GetChatMessage.
func assignModel(ctx context.Context, client *http.Client, auth *turnAuth, routerUID string, prompt *exav1.ChatMessagePrompt) (*modelAssignment, error) {
	req := &exav1.AssignModelRequest{
		Metadata:          cliSessionMetadata(auth.apiKey, ""),
		ModelRouterUid:    routerUID,
		CascadeId:         auth.cascadeID,
		ChatMessagePrompt: prompt,
	}
	resp := &exav1.AssignModelResponse{}
	if err := postUnary(ctx, client, auth.baseURL, assignModelPath, req, resp); err != nil {
		return nil, err
	}
	assignment := resp.GetAssignment()
	if assignment == nil || assignment.GetAssignmentJwt() == "" || assignment.GetModelUid() == "" {
		return nil, fmt.Errorf("Devin AssignModel error: response carried no assignment JWT and model uid")
	}
	return &modelAssignment{jwt: assignment.GetAssignmentJwt(), modelUID: assignment.GetModelUid()}, nil
}

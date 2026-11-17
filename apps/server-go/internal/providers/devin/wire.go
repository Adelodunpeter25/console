// Connect-RPC transport for the Devin Cascade API: unary calls use
// `application/proto` bodies, GetChatMessage uses Connect server-streaming
// envelopes (5-byte header: flags + big-endian length) with gzip. Response
// payloads may arrive gzipped per-frame; unary responses may be bare or
// gzipped protobuf (the edge varies), so both are attempted.
package devin

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"

	"google.golang.org/protobuf/proto"

	exav1 "github.com/Adelodunpeter25/console/apps/server-go/internal/gen/exa/v1"
)

// devinOS maps runtime.GOOS onto the Metadata.os vocabulary.
func devinOS() string {
	switch runtime.GOOS {
	case "darwin":
		return "darwin"
	case "windows":
		return "windows"
	default:
		return "linux"
	}
}

// cliIdentityFields returns the released-CLI Metadata fields ("chisel"):
// ideType "chisel" is what unlocks router assignment (AssignModel) and the
// CLI model surface; the older Windsurf identity does not reach them.
func cliIdentityFields(m *exav1.Metadata) {
	m.IdeName = "devin-cli"
	m.IdeType = "chisel"
	m.IdeVersion = "3000.11.3"
	m.ExtensionName = "chisel"
	m.ExtensionVersion = "3000.11.3"
	m.Locale = "en"
	m.Os = devinOS()
}

// discoveryIdentityFields is the dev-channel identity native discovery
// announces; it unlocks the full GetCliModelConfigs set where the released
// CLI identity gets a seed-only catalog on some seats.
func discoveryIdentityFields(m *exav1.Metadata) {
	m.IdeName = "chisel"
	m.IdeVersion = "0.0.0-dev"
	m.ExtensionName = "chisel"
	m.ExtensionVersion = "0.0.0-dev"
	m.Locale = "en"
	m.Os = devinOS()
}

// cliSessionMetadata is Metadata for released-CLI calls authenticated by a
// session token. apiKey must already be wire-encoded (see
// normalizeSessionToken); userJWT comes from GetUserJwt.
func cliSessionMetadata(apiKey, userJWT string) *exav1.Metadata {
	m := &exav1.Metadata{}
	cliIdentityFields(m)
	m.ApiKey = apiKey
	m.UserJwt = userJWT
	return m
}

// discoverySessionMetadata is Metadata for the dev-channel
// GetCliModelConfigs call. Asking for the internal display slots is what
// makes the server return its full catalog; the internal ones are filtered
// client-side, exactly as native does.
func discoverySessionMetadata(apiKey string) *exav1.Metadata {
	m := &exav1.Metadata{}
	discoveryIdentityFields(m)
	m.ApiKey = normalizeSessionToken(apiKey)
	m.SupportedModelDisplays = []exav1.DisplayOption{
		exav1.DisplayOption_DISPLAY_OPTION_MODEL_ROUTER,
		exav1.DisplayOption_DISPLAY_OPTION_QUICK_REVIEW,
		exav1.DisplayOption_DISPLAY_OPTION_INTERNAL_DEFAULT,
		exav1.DisplayOption_DISPLAY_OPTION_UNCLASSIFIED,
		exav1.DisplayOption_DISPLAY_OPTION_NORMAL,
	}
	return m
}

// legacyDiscoveryMetadata is the Windsurf-editor identity some enterprise
// seats require for their full credential-scoped roster (native chisel
// discovery returns a two-row seed there).
func legacyDiscoveryMetadata(apiKey string) *exav1.Metadata {
	m := &exav1.Metadata{}
	m.IdeName = "windsurf"
	m.IdeVersion = "3.2.23"
	m.ExtensionName = "windsurf"
	m.ExtensionVersion = "1.48.2"
	m.Locale = "en"
	m.ApiKey = apiKey
	return m
}

// postUnary POSTs a unary Connect RPC (`application/proto`) and decodes the
// response, trying bare protobuf first then gzip (edges vary).
func postUnary(ctx context.Context, client *http.Client, baseURL, path string, req, resp proto.Message) error {
	_, err := postUnaryStatus(ctx, client, baseURL, path, req, resp)
	return err
}

// postUnaryStatus is postUnary plus the HTTP status, for callers that
// branch on it (the GetUserJwt 401 session-token retry).
func postUnaryStatus(ctx context.Context, client *http.Client, baseURL, path string, req, resp proto.Message) (int, error) {
	body, err := proto.Marshal(req)
	if err != nil {
		return 0, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(baseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	httpReq.Header.Set("content-type", "application/proto")
	httpReq.Header.Set("connect-protocol-version", "1")
	httpReq.Header.Set("accept", "*/*")

	r, err := client.Do(httpReq)
	if err != nil {
		return 0, err
	}
	defer r.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(r.Body, 32<<20))
	if err != nil {
		return r.StatusCode, err
	}
	if r.StatusCode < 200 || r.StatusCode >= 300 {
		return r.StatusCode, devinError(path, r.StatusCode, r.Status, payload)
	}
	if err := proto.Unmarshal(payload, resp); err == nil {
		return r.StatusCode, nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		return r.StatusCode, fmt.Errorf("Devin %s: undecodable response (%d bytes)", path, len(payload))
	}
	raw, err := io.ReadAll(zr)
	if err != nil {
		return r.StatusCode, err
	}
	if err := proto.Unmarshal(raw, resp); err != nil {
		return r.StatusCode, fmt.Errorf("Devin %s: undecodable response", path)
	}
	return r.StatusCode, nil
}

// devinError formats an HTTP failure, preferring a body snippet.
func devinError(path string, status int, statusText string, payload []byte) error {
	msg := strings.TrimSpace(string(payload))
	if len(msg) > 500 {
		msg = msg[:500]
	}
	if msg == "" {
		return fmt.Errorf("Devin %s failed: %d %s", path, status, statusText)
	}
	return fmt.Errorf("Devin %s failed: %d %s: %s", path, status, statusText, msg)
}

// streamChat POSTs GetChatMessage as a Connect server-stream and invokes
// onMessage for each decoded GetChatMessageResponse. The request body is a
// single gzipped envelope; response envelopes may each be gzipped. The
// end-of-stream trailer carries JSON, possibly an {error:{code,message}}.
func streamChat(
	ctx context.Context,
	client *http.Client,
	baseURL, path string,
	req proto.Message,
	onMessage func(*exav1.GetChatMessageResponse) error,
) error {
	raw, err := proto.Marshal(req)
	if err != nil {
		return err
	}
	var gzBuf bytes.Buffer
	zw := gzip.NewWriter(&gzBuf)
	if _, err := zw.Write(raw); err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	frame := make([]byte, 5+gzBuf.Len())
	frame[0] = connectCompressedFlag
	frame[1] = byte(gzBuf.Len() >> 24)
	frame[2] = byte(gzBuf.Len() >> 16)
	frame[3] = byte(gzBuf.Len() >> 8)
	frame[4] = byte(gzBuf.Len())
	copy(frame[5:], gzBuf.Bytes())

	httpReq, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(baseURL, "/")+path, bytes.NewReader(frame))
	if err != nil {
		return err
	}
	httpReq.Header.Set("content-type", "application/connect+proto")
	httpReq.Header.Set("connect-protocol-version", "1")
	httpReq.Header.Set("connect-content-encoding", "gzip")
	httpReq.Header.Set("accept-encoding", "identity")
	httpReq.Header.Set("connect-accept-encoding", "gzip")
	httpReq.Header.Set("user-agent", "connect-go/1.18.1 (go1.26.3)")

	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		payload, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		return devinError(path, resp.StatusCode, resp.Status, payload)
	}

	var pending []byte
	buf := make([]byte, 64*1024)
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			pending = append(pending, buf[:n]...)
		}
		for len(pending) >= 5 {
			flag := pending[0]
			length := int(pending[1])<<24 | int(pending[2])<<16 | int(pending[3])<<8 | int(pending[4])
			if length > maxConnectFramePayload {
				return fmt.Errorf("Devin Connect frame length %d exceeds %d-byte cap", length, maxConnectFramePayload)
			}
			if len(pending) < 5+length {
				break
			}
			payload := pending[5 : 5+length]
			pending = pending[5+length:]

			body := payload
			if flag&connectCompressedFlag != 0 {
				zr, err := gzip.NewReader(bytes.NewReader(payload))
				if err != nil {
					return fmt.Errorf("Devin Connect frame gunzip: %w", err)
				}
				body, err = io.ReadAll(zr)
				if err != nil {
					return fmt.Errorf("Devin Connect frame gunzip: %w", err)
				}
			}
			if flag&connectEndStreamFlag != 0 {
				if terr := parseTrailerError(body); terr != nil {
					return terr
				}
				continue
			}
			msg := &exav1.GetChatMessageResponse{}
			if err := proto.Unmarshal(body, msg); err != nil {
				return fmt.Errorf("Devin GetChatMessageResponse decode: %w", err)
			}
			if err := onMessage(msg); err != nil {
				return err
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				return nil
			}
			return readErr
		}
	}
}

// parseTrailerError extracts {error:{code,message}} from an end-of-stream
// JSON trailer; nil when absent or unparseable.
func parseTrailerError(body []byte) error {
	text := strings.TrimSpace(string(body))
	if text == "" {
		return nil
	}
	var trailer struct {
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(text), &trailer); err != nil || trailer.Error == nil {
		return nil
	}
	code := trailer.Error.Code
	message := trailer.Error.Message
	if code == "" && message == "" {
		return nil
	}
	return &ConnectStreamError{Code: code, Message: message}
}

// ConnectStreamError is a Connect end-of-stream trailer rejection. These
// carry no HTTP error body, so the trailer is the only server evidence.
type ConnectStreamError struct {
	Code    string
	Message string
}

func (e *ConnectStreamError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("Devin stream error %s: %s", e.Code, e.Message)
	}
	return "Devin stream error: " + e.Message
}

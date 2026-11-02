// Package msgparts splits a stored message into a small skeleton plus its heavy
// bodies, and puts it back together.
//
// A message row used to be one JSON document. Two thirds of the bytes in a real
// database were tool output and arguments, each JSON wrapped in base64 wrapped
// in more JSON, plus pasted images, so reading words meant loading megabytes.
// Splitting moves each heavy body into its own row, stored as the raw bytes it
// really is, and leaves a skeleton that still describes the whole message.
//
// The one rule: Assemble(Split(x)) must be the same message as x. The skeleton
// keeps every field the splitter does not recognise, so a field added to the
// message format later is carried through untouched instead of silently lost.
// Equal compares two messages by meaning (key order and whitespace ignored),
// which is what the migration and the read path use to prove a rebuild is right.
package msgparts

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

// Kind says what a part body is.
const (
	KindToolArgs   = "tool_args"   // a tool call's arguments (JSON bytes)
	KindToolResult = "tool_result" // a tool result's content (JSON bytes)
	KindImage      = "image"       // a pasted image attachment
	KindScreenshot = "screenshot"  // a browser annotation's screenshot
)

// Part is one extracted body.
type Part struct {
	// Slot locates the body in the skeleton: the part's position among all parts
	// of the message, in the order Split found them.
	Slot int
	Kind string
	// Meta carries what is needed to put the body back that is not the body
	// itself: its mime type, and how it was encoded (see Encoding).
	Meta Meta
	// Body is the raw bytes. For an image it is the decoded image, not base64.
	Body []byte
}

// Meta describes a part.
type Meta struct {
	Mime string `json:"mime,omitempty"`
	// Encoding is how Body relates to the original JSON string:
	//   "base64"  the original string was base64 of Body (the normal case)
	//   "text"    the original string was not valid base64, so Body is the string
	//             itself and is written back as-is
	Encoding string `json:"enc"`
	// Ref names the file holding Body when the body is stored outside the
	// database (an image). It is the lowercase hex SHA-256 of Body. The codec
	// itself never reads it: the storage layer fills Body from Ref before
	// Assemble.
	Ref string `json:"ref,omitempty"`
}

// Result is what splitting one message produces.
type Result struct {
	// Skeleton is the message JSON with every extracted body replaced by a slot
	// marker. It is small, valid JSON, and contains all the fields Split did not
	// extract.
	Skeleton []byte
	Parts    []Part
	// Text is the human-readable words in the message: what a person wrote, what
	// the assistant said, and a short note per tool call. It feeds search.
	Text string
}

// slotKey is the marker object that stands where a body was.
const slotKey = "$part"

// Split extracts the heavy bodies from one stored message.
func Split(content []byte) (*Result, error) {
	var root map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(content))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("message is not a JSON object: %w", err)
	}
	if dec.More() {
		return nil, fmt.Errorf("message has trailing data")
	}

	out := &Result{}
	var words []string
	add := func(s string) {
		if s = strings.TrimSpace(s); s != "" {
			words = append(words, s)
		}
	}

	next := func(kind string, raw string, mime string) json.RawMessage {
		meta := Meta{Mime: mime, Encoding: "base64"}
		body, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			// Not base64: keep the string itself so it can be put back exactly.
			meta.Encoding = "text"
			body = []byte(raw)
		}
		slot := len(out.Parts)
		out.Parts = append(out.Parts, Part{Slot: slot, Kind: kind, Meta: meta, Body: body})
		return marker(slot)
	}

	if raw, ok := root["assistant"]; ok {
		var a map[string]json.RawMessage
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, fmt.Errorf("assistant: %w", err)
		}
		if rc, ok := a["content"]; ok {
			var items []map[string]json.RawMessage
			if err := json.Unmarshal(rc, &items); err != nil {
				return nil, fmt.Errorf("assistant.content: %w", err)
			}
			for _, item := range items {
				if t, ok := item["text"]; ok {
					var x struct {
						Text string `json:"text"`
					}
					if json.Unmarshal(t, &x) == nil {
						add(x.Text)
					}
				}
				if tc, ok := item["toolCall"]; ok {
					var call map[string]json.RawMessage
					if err := json.Unmarshal(tc, &call); err != nil {
						return nil, fmt.Errorf("toolCall: %w", err)
					}
					if args, ok := call["arguments"]; ok {
						var s string
						if json.Unmarshal(args, &s) == nil && s != "" {
							add(toolNote(call, s))
							call["arguments"] = next(KindToolArgs, s, "")
							item["toolCall"] = mustMarshal(call)
						}
					}
				}
			}
			a["content"] = mustMarshal(items)
		}
		root["assistant"] = mustMarshal(a)
	}

	if raw, ok := root["toolResult"]; ok {
		var tr map[string]json.RawMessage
		if err := json.Unmarshal(raw, &tr); err != nil {
			return nil, fmt.Errorf("toolResult: %w", err)
		}
		if rr, ok := tr["results"]; ok {
			var results []map[string]json.RawMessage
			if err := json.Unmarshal(rr, &results); err != nil {
				return nil, fmt.Errorf("toolResult.results: %w", err)
			}
			for _, res := range results {
				if c, ok := res["content"]; ok {
					var s string
					if json.Unmarshal(c, &s) == nil && s != "" {
						res["content"] = next(KindToolResult, s, "")
					}
				}
			}
			tr["results"] = mustMarshal(results)
		}
		root["toolResult"] = mustMarshal(tr)
	}

	if raw, ok := root["user"]; ok {
		var u map[string]json.RawMessage
		if err := json.Unmarshal(raw, &u); err != nil {
			return nil, fmt.Errorf("user: %w", err)
		}
		if c, ok := u["content"]; ok {
			var s string
			if json.Unmarshal(c, &s) == nil {
				add(s)
			}
		}
		if ra, ok := u["attachments"]; ok {
			var atts []map[string]json.RawMessage
			if err := json.Unmarshal(ra, &atts); err != nil {
				return nil, fmt.Errorf("user.attachments: %w", err)
			}
			for _, att := range atts {
				if d, ok := att["data"]; ok {
					var s string
					if json.Unmarshal(d, &s) == nil && s != "" {
						mime := ""
						if m, ok := att["mimeType"]; ok {
							_ = json.Unmarshal(m, &mime)
						}
						att["data"] = next(KindImage, s, mime)
					}
				}
			}
			u["attachments"] = mustMarshal(atts)
		}
		if ra, ok := u["annotations"]; ok {
			var anns []map[string]json.RawMessage
			if err := json.Unmarshal(ra, &anns); err != nil {
				return nil, fmt.Errorf("user.annotations: %w", err)
			}
			for _, ann := range anns {
				if d, ok := ann["screenshotBase64"]; ok {
					var s string
					if json.Unmarshal(d, &s) == nil && s != "" {
						ann["screenshotBase64"] = next(KindScreenshot, s, "")
					}
				}
			}
			u["annotations"] = mustMarshal(anns)
		}
		root["user"] = mustMarshal(u)
	}

	sk, err := json.Marshal(root)
	if err != nil {
		return nil, err
	}
	out.Skeleton = sk
	out.Text = strings.Join(words, "\n")
	return out, nil
}

// Assemble puts a message back together from its skeleton and parts.
func Assemble(skeleton []byte, parts []Part) ([]byte, error) {
	bySlot := make(map[int]Part, len(parts))
	for _, p := range parts {
		bySlot[p.Slot] = p
	}
	var root any
	dec := json.NewDecoder(bytes.NewReader(skeleton))
	dec.UseNumber()
	if err := dec.Decode(&root); err != nil {
		return nil, fmt.Errorf("skeleton: %w", err)
	}
	used := 0
	var firstErr error
	var walk func(v any) any
	walk = func(v any) any {
		switch t := v.(type) {
		case map[string]any:
			if len(t) == 1 {
				if slot, ok := t[slotKey]; ok {
					n, isNum := slot.(json.Number)
					idx, err := n.Int64()
					if isNum && err == nil {
						p, found := bySlot[int(idx)]
						if !found {
							if firstErr == nil {
								firstErr = fmt.Errorf("skeleton refers to part %d, which is missing", idx)
							}
							return v
						}
						used++
						return restore(p)
					}
				}
			}
			for k, x := range t {
				t[k] = walk(x)
			}
			return t
		case []any:
			for i, x := range t {
				t[i] = walk(x)
			}
			return t
		}
		return v
	}
	root = walk(root)
	if firstErr != nil {
		return nil, firstErr
	}
	if used != len(parts) {
		return nil, fmt.Errorf("skeleton used %d of %d parts", used, len(parts))
	}
	return json.Marshal(root)
}

func restore(p Part) string {
	if p.Meta.Encoding == "text" {
		return string(p.Body)
	}
	return base64.StdEncoding.EncodeToString(p.Body)
}

func marker(slot int) json.RawMessage {
	return json.RawMessage(fmt.Sprintf(`{"%s":%d}`, slotKey, slot))
}

func mustMarshal(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err) // marshalling values just decoded from JSON cannot fail
	}
	return b
}

// toolNote is the one line a tool call contributes to searchable text: its name
// and its main argument, so "read_file src/auth.go" is findable without decoding
// the whole body.
func toolNote(call map[string]json.RawMessage, argsB64 string) string {
	var name string
	_ = json.Unmarshal(call["name"], &name)
	body, err := base64.StdEncoding.DecodeString(argsB64)
	if err != nil {
		return name
	}
	var args map[string]any
	if json.Unmarshal(body, &args) != nil {
		return name
	}
	for _, key := range []string{"path", "filePath", "command", "pattern", "query", "url", "name"} {
		if v, ok := args[key].(string); ok && strings.TrimSpace(v) != "" {
			v = strings.TrimSpace(v)
			if len(v) > 200 {
				v = v[:200]
			}
			return name + " " + v
		}
	}
	return name
}

// Equal reports whether two message documents mean the same thing: same keys,
// same values, ignoring key order and whitespace. Numbers compare by value.
func Equal(a, b []byte) (bool, error) {
	var x, y any
	da := json.NewDecoder(bytes.NewReader(a))
	da.UseNumber()
	if err := da.Decode(&x); err != nil {
		return false, fmt.Errorf("first: %w", err)
	}
	db := json.NewDecoder(bytes.NewReader(b))
	db.UseNumber()
	if err := db.Decode(&y); err != nil {
		return false, fmt.Errorf("second: %w", err)
	}
	return reflect.DeepEqual(x, y), nil
}

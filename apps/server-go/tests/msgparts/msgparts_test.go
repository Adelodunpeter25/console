// The codec's one promise: Assemble(Split(x)) means the same as x. These tests
// attack it with every message shape, awkward encodings and unknown fields, and
// then run it over every real message in the developer's own databases.
package tests

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/msgparts"
)

func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }

func roundTrip(t *testing.T, name, doc string) *msgparts.Result {
	t.Helper()
	res, err := msgparts.Split([]byte(doc))
	if err != nil {
		t.Fatalf("%s: split: %v", name, err)
	}
	back, err := msgparts.Assemble(res.Skeleton, res.Parts)
	if err != nil {
		t.Fatalf("%s: assemble: %v", name, err)
	}
	same, err := msgparts.Equal([]byte(doc), back)
	if err != nil || !same {
		t.Fatalf("%s: round trip changed the message\n  before: %.300s\n  after:  %.300s\n  err: %v", name, doc, back, err)
	}
	return res
}

func TestRoundTripEveryMessageShape(t *testing.T) {
	cases := map[string]string{
		"user text only":    `{"user":{"content":"hello"},"id":"u1","createdAt":"1700000000000"}`,
		"user synthetic":    `{"user":{"content":"x","synthetic":true}}`,
		"user context":      `{"user":{"content":"x","contextFiles":["a/b","c"]}}`,
		"user image":        `{"user":{"content":"look","attachments":[{"data":"` + b64("\x89PNG\r\n\x1a\nbytes") + `","mimeType":"image/png"}]}}`,
		"user two images":   `{"user":{"content":"","attachments":[{"data":"` + b64("one") + `","mimeType":"image/png"},{"data":"` + b64("two") + `","mimeType":"image/jpeg"}]}}`,
		"user annotation":   `{"user":{"content":"x","annotations":[{"id":"a","url":"u","selector":"s","screenshotBase64":"` + b64("shot") + `","computedStyles":{"k":"v"}}]}}`,
		"assistant text":    `{"assistant":{"id":"m1","content":[{"text":{"text":"Hi","thoughtSignature":"sig"}}],"stopReason":"stop","tokPerSec":42.5}}`,
		"assistant think":   `{"assistant":{"id":"m1","content":[{"thinking":{"text":"hmm"}},{"text":{"text":"ok"}}],"stopReason":"stop"}}`,
		"assistant call":    `{"assistant":{"id":"m1","content":[{"toolCall":{"id":"c1","name":"read_file","arguments":"` + b64(`{"path":"/a"}`) + `","label":"Read File","kindFlags":12,"icon":"file","thoughtSignature":"s"}}],"stopReason":"toolUse"}}`,
		"assistant calls":   `{"assistant":{"id":"m1","content":[{"text":{"text":"x"}},{"toolCall":{"id":"c1","name":"a","arguments":"` + b64(`{}`) + `"}},{"toolCall":{"id":"c2","name":"b","arguments":"` + b64(`{"x":1}`) + `"}}],"stopReason":"toolUse"}}`,
		"call no args":      `{"assistant":{"id":"m1","content":[{"toolCall":{"id":"c1","name":"x"}}],"stopReason":"toolUse"}}`,
		"call empty args":   `{"assistant":{"id":"m1","content":[{"toolCall":{"id":"c1","name":"x","arguments":""}}],"stopReason":"toolUse"}}`,
		"assistant image":   `{"assistant":{"id":"m1","content":[{"image":{"data":"` + b64("img") + `","mimeType":"image/png"}}],"stopReason":"stop"}}`,
		"empty content":     `{"assistant":{"id":"m1","content":[],"stopReason":"stop"}}`,
		"tool result":       `{"toolResult":{"results":[{"toolCallId":"c1","toolName":"bash","content":"` + b64(`[{"type":"text","text":"out"}]`) + `"}]}}`,
		"tool result err":   `{"toolResult":{"results":[{"toolCallId":"c1","toolName":"bash","content":"` + b64(`"boom"`) + `","isError":true}]}}`,
		"tool results two":  `{"toolResult":{"results":[{"toolCallId":"c1","content":"` + b64(`1`) + `"},{"toolCallId":"c2","content":"` + b64(`2`) + `"}]}}`,
		"result visual":     `{"toolResult":{"results":[{"toolCallId":"c1","toolName":"visualize","content":"` + b64(`x`) + `","visual":{"version":1,"spec":"` + b64(`{"v":1}`) + `"}}]}}`,
		"result no content": `{"toolResult":{"results":[{"toolCallId":"c1","toolName":"x"}]}}`,
		"empty results":     `{"toolResult":{"results":[]}}`,
		"unicode":           `{"user":{"content":"héllo 🔥 日本語 \u2028 \"quoted\" \\ back"}}`,
		"big numbers":       `{"user":{"content":"x"},"createdAt":"9007199254740993","n":12345678901234567890}`,
		"floats":            `{"assistant":{"id":"m","content":[],"stopReason":"stop","tokPerSec":0.1}}`,
		"empty user":        `{"user":{}}`,
		// A field this codec has never heard of must survive.
		"unknown top":       `{"user":{"content":"x"},"futureField":{"nested":[1,2,3]}}`,
		"unknown inner":     `{"user":{"content":"x","newThing":true}}`,
		"unknown in part":   `{"assistant":{"id":"m","content":[{"toolCall":{"id":"c","name":"n","arguments":"` + b64(`{}`) + `","futureKey":9}}],"stopReason":"stop"}}`,
		"unknown part kind": `{"assistant":{"id":"m","content":[{"hologram":{"blob":"abc"}}],"stopReason":"stop"}}`,
		"unknown role":      `{"mystery":{"content":"zz"}}`,
	}
	for name, doc := range cases {
		roundTrip(t, name, doc)
	}
}

// A string field that is not valid base64 must come back exactly, not be
// "repaired" or dropped.
func TestRoundTripNonBase64BodiesComeBackExactly(t *testing.T) {
	for name, body := range map[string]string{
		"plain text":  "this is not base64!",
		"bad padding": "YWJj=",
		"url safe":    "a-b_c",
		"whitespace":  "YWJj ZGVm",
		"unicode":     "héllo",
	} {
		doc := `{"toolResult":{"results":[{"toolCallId":"c","content":` + mustJSON(body) + `}]}}`
		res := roundTrip(t, name, doc)
		if len(res.Parts) != 1 || res.Parts[0].Meta.Encoding != "text" {
			t.Errorf("%s: expected one text-encoded part, got %+v", name, res.Parts)
		}
	}
}

// Valid base64 whose padding the encoder would not reproduce ("YQ" vs "YQ==")
// is not canonical; it must round trip as it was written.
func TestRoundTripNonCanonicalBase64(t *testing.T) {
	doc := `{"toolResult":{"results":[{"toolCallId":"c","content":"YQ"}]}}`
	res, err := msgparts.Split([]byte(doc))
	if err != nil {
		t.Fatal(err)
	}
	back, err := msgparts.Assemble(res.Skeleton, res.Parts)
	if err != nil {
		t.Fatal(err)
	}
	if same, _ := msgparts.Equal([]byte(doc), back); !same {
		t.Fatalf("non-canonical base64 was rewritten: %s", back)
	}
}

func TestSplitMovesTheHeavyBodiesOut(t *testing.T) {
	big := strings.Repeat("x", 100_000)
	doc := `{"toolResult":{"results":[{"toolCallId":"c1","toolName":"bash","content":"` + b64(big) + `"}]}}`
	res := roundTrip(t, "big result", doc)
	if len(res.Skeleton) > 400 {
		t.Fatalf("the skeleton still carries the body: %d bytes", len(res.Skeleton))
	}
	if len(res.Parts) != 1 || res.Parts[0].Kind != msgparts.KindToolResult || string(res.Parts[0].Body) != big {
		t.Fatalf("the body was not stored raw: %+v", res.Parts[0].Kind)
	}
	// Raw storage is smaller than base64.
	if len(res.Parts[0].Body) >= len(b64(big)) {
		t.Fatal("a raw body should be smaller than its base64 form")
	}
}

func TestSplitExtractsSearchableText(t *testing.T) {
	user := roundTrip(t, "u", `{"user":{"content":"fix the login bug"}}`)
	if user.Text != "fix the login bug" {
		t.Fatalf("user text = %q", user.Text)
	}
	asst := roundTrip(t, "a", `{"assistant":{"id":"m","content":[`+
		`{"thinking":{"text":"private musings"}},`+
		`{"text":{"text":"I will look at the auth module."}},`+
		`{"toolCall":{"id":"c","name":"read_file","arguments":"`+b64(`{"path":"internal/auth/login.go"}`)+`"}},`+
		`{"toolCall":{"id":"d","name":"bash","arguments":"`+b64(`{"command":"go test ./..."}`)+`"}}`+
		`],"stopReason":"toolUse"}}`)
	for _, want := range []string{"I will look at the auth module.", "read_file internal/auth/login.go", "bash go test ./..."} {
		if !strings.Contains(asst.Text, want) {
			t.Errorf("searchable text is missing %q: %q", want, asst.Text)
		}
	}
	if strings.Contains(asst.Text, "private musings") {
		t.Error("thinking must not be searchable by default")
	}
	// Tool output and images are not words a person said.
	res := roundTrip(t, "r", `{"toolResult":{"results":[{"toolCallId":"c","content":"`+b64("SECRET-OUTPUT")+`"}]}}`)
	if res.Text != "" {
		t.Fatalf("tool output must not be in searchable text: %q", res.Text)
	}
	img := roundTrip(t, "i", `{"user":{"content":"see","attachments":[{"data":"`+b64("PIXELS")+`","mimeType":"image/png"}]}}`)
	if img.Text != "see" {
		t.Fatalf("image data leaked into text: %q", img.Text)
	}
	if len(img.Parts) != 1 || img.Parts[0].Kind != msgparts.KindImage || img.Parts[0].Meta.Mime != "image/png" {
		t.Fatalf("image part = %+v", img.Parts)
	}
}

func TestSplitRejectsBadInput(t *testing.T) {
	for name, doc := range map[string]string{
		"not json":  `not json at all`,
		"array":     `[1,2,3]`,
		"empty":     ``,
		"trailing":  `{"user":{}} {"x":1}`,
		"bad inner": `{"assistant":"a string"}`,
		"bad parts": `{"assistant":{"content":"not an array"}}`,
	} {
		if _, err := msgparts.Split([]byte(doc)); err == nil {
			t.Errorf("%s: expected an error, not a silent pass", name)
		}
	}
}

func TestAssembleRefusesAnInconsistentSet(t *testing.T) {
	res, err := msgparts.Split([]byte(`{"toolResult":{"results":[{"toolCallId":"c","content":"` + b64("x") + `"}]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := msgparts.Assemble(res.Skeleton, nil); err == nil {
		t.Error("a missing part must be an error, not a message with a hole")
	}
	extra := append(append([]msgparts.Part(nil), res.Parts...), msgparts.Part{Slot: 9, Kind: "x", Meta: msgparts.Meta{Encoding: "base64"}, Body: []byte("y")})
	if _, err := msgparts.Assemble(res.Skeleton, extra); err == nil {
		t.Error("a part the skeleton does not use must be an error")
	}
}

func TestEqualIgnoresOrderAndWhitespaceButNotValues(t *testing.T) {
	if same, _ := msgparts.Equal([]byte(`{"a":1,"b":[1,2]}`), []byte("{ \"b\": [1, 2],\n \"a\": 1 }")); !same {
		t.Error("key order and whitespace must not matter")
	}
	for name, pair := range map[string][2]string{
		"value":  {`{"a":1}`, `{"a":2}`},
		"key":    {`{"a":1}`, `{"b":1}`},
		"array":  {`{"a":[1,2]}`, `{"a":[2,1]}`},
		"extra":  {`{"a":1}`, `{"a":1,"b":2}`},
		"string": {`{"a":"x"}`, `{"a":"y"}`},
	} {
		if same, _ := msgparts.Equal([]byte(pair[0]), []byte(pair[1])); same {
			t.Errorf("%s: different messages compared equal", name)
		}
	}
}

func mustJSON(s string) string { b, _ := json.Marshal(s); return string(b) }

// The gate. Every message in every session database under the developer's own
// console storage is split and rebuilt, and each must come back identical. A
// failure here is a field the codec would have lost from real chat history.
// Skipped when there is no real storage, so CI and fresh clones are unaffected.
func TestRoundTripOverRealMessages(t *testing.T) {
	store := os.Getenv("CONSOLE_STORAGE_DIR")
	if store == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			t.Skip("no home directory")
		}
		store = filepath.Join(home, ".console")
	}
	var dbs []string
	_ = filepath.Walk(store, func(p string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() && strings.HasSuffix(p, ".db") && strings.Contains(p, string(filepath.Separator)+"sessions"+string(filepath.Separator)) {
			dbs = append(dbs, p)
		}
		return nil
	})
	if len(dbs) == 0 {
		t.Skip("no session databases under " + store)
	}
	total, failed, skipped := 0, 0, 0
	for _, path := range dbs {
		// Read-only, so running the test can never touch real history.
		conn, err := sql.Open("sqlite", "file:"+path+"?mode=ro&immutable=1")
		if err != nil {
			skipped++
			continue
		}
		rows, err := conn.Query(`SELECT id, content FROM messages`)
		if err != nil {
			conn.Close()
			skipped++
			continue
		}
		for rows.Next() {
			var id, content string
			if rows.Scan(&id, &content) != nil {
				continue
			}
			total++
			res, err := msgparts.Split([]byte(content))
			if err != nil {
				failed++
				t.Errorf("%s message %s: split: %v", filepath.Base(path), id, err)
				continue
			}
			back, err := msgparts.Assemble(res.Skeleton, res.Parts)
			if err != nil {
				failed++
				t.Errorf("%s message %s: assemble: %v", filepath.Base(path), id, err)
				continue
			}
			if same, _ := msgparts.Equal([]byte(content), back); !same {
				failed++
				t.Errorf("%s message %s: rebuilt message differs (%d bytes before, %d after)", filepath.Base(path), id, len(content), len(back))
			}
		}
		rows.Close()
		conn.Close()
	}
	t.Logf("real data: %d messages across %d databases (%d unreadable, skipped), %d failed", total, len(dbs), skipped, failed)
	if total == 0 {
		t.Skip("no messages found")
	}
}

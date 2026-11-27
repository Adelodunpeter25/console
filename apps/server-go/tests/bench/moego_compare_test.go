package tests

import (
	stdb64 "encoding/base64"
	stdjson "encoding/json"
	stdre "regexp"
	"strings"
	"testing"

	moeb64 "github.com/MoeForge/moego/base64"
	moejson "github.com/MoeForge/moego/json"
	moere "github.com/MoeForge/moego/regexp"
)

type msg struct {
	ID    string            `json:"id"`
	Role  string            `json:"role"`
	Parts []part            `json:"parts"`
	Meta  map[string]string `json:"meta"`
}

type part struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
	Data string `json:"data,omitempty"`
}

func small() msg {
	return msg{ID: "c1564b9f-5976-cb28", Role: "assistant",
		Parts: []part{{Type: "text", Text: strings.Repeat("hello world ", 40)}, {Type: "tool", Text: "readFile"}},
		Meta:  map[string]string{"model": "m", "provider": "codex"}}
}

func big() msg {
	raw := make([]byte, 1<<20)
	for i := range raw {
		raw[i] = byte(i * 7)
	}
	return msg{ID: "x", Role: "user", Parts: []part{{Type: "image", Data: stdb64.StdEncoding.EncodeToString(raw)}}}
}

func benchJSON(b *testing.B, v msg, enc func(any) ([]byte, error), dec func([]byte, any) error) {
	data, _ := enc(v)
	b.SetBytes(int64(len(data)))
	b.Run("marshal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			enc(v)
		}
	})
	b.Run("unmarshal", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			var m msg
			if err := dec(data, &m); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkJSONSmallStd(b *testing.B) { benchJSON(b, small(), stdjson.Marshal, stdjson.Unmarshal) }
func BenchmarkJSONSmallMoe(b *testing.B) { benchJSON(b, small(), moejson.Marshal, moejson.Unmarshal) }
func BenchmarkJSONBigStd(b *testing.B)   { benchJSON(b, big(), stdjson.Marshal, stdjson.Unmarshal) }
func BenchmarkJSONBigMoe(b *testing.B)   { benchJSON(b, big(), moejson.Marshal, moejson.Unmarshal) }

var raw1m = make([]byte, 1<<20)

func BenchmarkB64EncStd(b *testing.B) {
	b.SetBytes(1 << 20)
	for i := 0; i < b.N; i++ {
		stdb64.StdEncoding.EncodeToString(raw1m)
	}
}
func BenchmarkB64EncMoe(b *testing.B) {
	b.SetBytes(1 << 20)
	for i := 0; i < b.N; i++ {
		moeb64.StdEncoding.EncodeToString(raw1m)
	}
}
func BenchmarkB64DecStd(b *testing.B) {
	s := stdb64.StdEncoding.EncodeToString(raw1m)
	b.SetBytes(1 << 20)
	for i := 0; i < b.N; i++ {
		stdb64.StdEncoding.DecodeString(s)
	}
}
func BenchmarkB64DecMoe(b *testing.B) {
	s := stdb64.StdEncoding.EncodeToString(raw1m)
	b.SetBytes(1 << 20)
	for i := 0; i < b.N; i++ {
		moeb64.StdEncoding.DecodeString(s)
	}
}

var text = strings.Repeat("error: failed to open /home/user/file.go:42 token sk-abc123DEF456 ok\n", 500)

func BenchmarkReStd(b *testing.B) {
	re := stdre.MustCompile(`sk-[A-Za-z0-9]{6,}|[\w/.]+\.go:\d+`)
	b.SetBytes(int64(len(text)))
	for i := 0; i < b.N; i++ {
		re.FindAllString(text, -1)
	}
}
func BenchmarkReMoe(b *testing.B) {
	re := moere.MustCompile(`sk-[A-Za-z0-9]{6,}|[\w/.]+\.go:\d+`)
	b.SetBytes(int64(len(text)))
	for i := 0; i < b.N; i++ {
		re.FindAllString(text, -1)
	}
}

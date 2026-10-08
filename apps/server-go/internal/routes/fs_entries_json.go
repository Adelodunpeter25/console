// Hand-written JSON encoder for GET /api/fs/entries.
//
// The listing is flat (clients rebuild the tree from each entry's path), and
// it is re-requested on every filesystem change, so the generic path
// (protojson per entry, then json.Marshal re-validating the result, then the
// envelope) was the dominant CPU cost on large trees. FsTreeEntry has four
// wire fields, so this writes the whole `{success,data}` response into one
// buffer. Output matches protojson's canonical shape: lowerCamel keys, `size`
// as a quoted decimal string, and default values (empty strings, false
// isDir) omitted. `children` is intentionally never written; see
// tests/api/fs_entries_json_test.go for the parity checks.
package routes

import (
	"strconv"
	"unicode/utf8"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/types"
)

// EncodeFsEntriesResponse returns the complete JSON body for /api/fs/entries.
func EncodeFsEntriesResponse(entries []types.FsTreeEntry) []byte {
	buf := make([]byte, 0, 64+len(entries)*128)
	buf = append(buf, `{"success":true,"data":[`...)
	for i := range entries {
		if i > 0 {
			buf = append(buf, ',')
		}
		buf = appendFsEntryJSON(buf, &entries[i])
	}
	return append(buf, ']', '}')
}

func appendFsEntryJSON(buf []byte, e *types.FsTreeEntry) []byte {
	buf = append(buf, '{')
	comma := false
	if e.Name != "" {
		buf = append(buf, `"name":`...)
		buf = appendJSONString(buf, e.Name)
		comma = true
	}
	if e.Path != "" {
		if comma {
			buf = append(buf, ',')
		}
		buf = append(buf, `"path":`...)
		buf = appendJSONString(buf, e.Path)
		comma = true
	}
	if e.IsDir {
		if comma {
			buf = append(buf, ',')
		}
		buf = append(buf, `"isDir":true`...)
		comma = true
	}
	if e.Size != nil {
		if comma {
			buf = append(buf, ',')
		}
		// protojson encodes uint64 as a quoted decimal string.
		buf = append(buf, `"size":"`...)
		buf = strconv.AppendUint(buf, uint64(*e.Size), 10)
		buf = append(buf, '"')
	}
	return append(buf, '}')
}

const jsonHex = "0123456789abcdef"

// appendJSONString appends s as a JSON string, escaping <, >, &, U+2028 and
// U+2029 like encoding/json does so output is byte-identical to the previous
// path. Bytes that are not valid UTF-8
// (legal in Unix filenames) become U+FFFD so one odd filename cannot fail the
// whole listing; the old protojson path returned a 500 for the entire request.
func appendJSONString(buf []byte, s string) []byte {
	buf = append(buf, '"')
	start := 0
	for i := 0; i < len(s); {
		c := s[i]
		if c < utf8.RuneSelf {
			if c >= ' ' && c != '"' && c != '\\' && c != '<' && c != '>' && c != '&' {
				i++
				continue
			}
			buf = append(buf, s[start:i]...)
			switch c {
			case '"', '\\':
				buf = append(buf, '\\', c)
			case '\n':
				buf = append(buf, '\\', 'n')
			case '\r':
				buf = append(buf, '\\', 'r')
			case '\t':
				buf = append(buf, '\\', 't')
			case '\b':
				buf = append(buf, '\\', 'b')
			case '\f':
				buf = append(buf, '\\', 'f')
			default: // control chars and <, >, & as \u00XX
				buf = append(buf, '\\', 'u', '0', '0', jsonHex[c>>4], jsonHex[c&0xf])
			}
			i++
			start = i
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			buf = append(buf, s[start:i]...)
			buf = append(buf, `\ufffd`...)
			i += size
			start = i
			continue
		}
		if r == '\u2028' || r == '\u2029' {
			buf = append(buf, s[start:i]...)
			buf = append(buf, '\\', 'u', '2', '0', '2', jsonHex[r&0xf])
			i += size
			start = i
			continue
		}
		i += size
	}
	buf = append(buf, s[start:]...)
	return append(buf, '"')
}

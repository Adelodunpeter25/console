package routes

import "unicode/utf8"

// sanitizeUTF8 drops invalid bytes so JSON frames always carry valid text.
func sanitizeUTF8(data []byte) string {
	if utf8.Valid(data) {
		return string(data)
	}
	out := make([]rune, 0, len(data))
	for len(data) > 0 {
		r, size := utf8.DecodeRune(data)
		if r != utf8.RuneError || size > 1 {
			out = append(out, r)
		}
		data = data[size:]
	}
	return string(out)
}

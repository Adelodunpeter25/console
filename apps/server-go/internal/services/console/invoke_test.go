package console

import "testing"

func TestSplitInvocation(t *testing.T) {
	cases := []struct {
		input, rest string
		ok          bool
	}{
		{"/console", "", true},
		{"  /console  ", "", true},
		{"/console inspect this", "inspect this", true},
		{"/consolex", "/consolex", false},
		{"please /console", "please /console", false},
	}
	for _, tc := range cases {
		rest, ok := SplitInvocation(tc.input)
		if rest != tc.rest || ok != tc.ok {
			t.Errorf("SplitInvocation(%q) = (%q, %v), want (%q, %v)", tc.input, rest, ok, tc.rest, tc.ok)
		}
	}
}

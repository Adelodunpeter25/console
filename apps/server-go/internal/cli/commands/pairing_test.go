package commands

import "testing"

// The host field a client is given assumes http when it carries no scheme,
// so the scheme is only ever dropped when it is the default one. An https
// address that loses its scheme is spoken to in the wrong protocol.
func TestHostPortKeepsANonDefaultScheme(t *testing.T) {
	cases := []struct {
		name string
		base string
		want string
	}{
		{
			name: "http keeps the short host and port",
			base: "http://192.168.1.10:3000",
			want: "192.168.1.10:3000",
		},
		{
			name: "https keeps its scheme",
			base: "https://k7m2xq9pa4dn.opentunnel.xyz",
			want: "https://k7m2xq9pa4dn.opentunnel.xyz",
		},
		{
			name: "https with a port keeps both",
			base: "https://server.example:8443",
			want: "https://server.example:8443",
		},
		{
			name: "a trailing slash is dropped",
			base: "https://server.example/",
			want: "https://server.example",
		},
		{
			name: "http with a trailing slash keeps the short form",
			base: "http://192.168.1.10:3000/",
			want: "192.168.1.10:3000",
		},
		{
			name: "a bare host is left alone",
			base: "192.168.1.10:3000",
			want: "192.168.1.10:3000",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := hostPort(tc.base); got != tc.want {
				t.Errorf("hostPort(%q) = %q, want %q", tc.base, got, tc.want)
			}
		})
	}
}

// A server behind a tunnel, proxy or load balancer is not reachable at the
// address it binds, so the operator states the address clients should use.
func TestPublicBaseURLHonoursTheOverride(t *testing.T) {
	t.Setenv("CONSOLE_PUBLIC_URL", "https://k7m2xq9pa4dn.opentunnel.xyz")
	got := publicBaseURL(t.TempDir())
	if got != "https://k7m2xq9pa4dn.opentunnel.xyz" {
		t.Errorf("publicBaseURL = %q", got)
	}
}

func TestPublicBaseURLDropsATrailingSlashFromTheOverride(t *testing.T) {
	t.Setenv("CONSOLE_PUBLIC_URL", "https://server.example/")
	if got := publicBaseURL(t.TempDir()); got != "https://server.example" {
		t.Errorf("publicBaseURL = %q", got)
	}
}

// Without an override the address falls back to the machine's own, which is
// plain http.
func TestPublicBaseURLFallsBackToTheMachine(t *testing.T) {
	t.Setenv("CONSOLE_PUBLIC_URL", "")
	t.Setenv("HOST", "192.168.1.10")
	t.Setenv("PORT", "3000")
	got := publicBaseURL(t.TempDir())
	if got != "http://192.168.1.10:3000" {
		t.Errorf("publicBaseURL = %q", got)
	}
}

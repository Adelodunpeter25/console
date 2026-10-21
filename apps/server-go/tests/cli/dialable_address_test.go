// Pairing address selection: the default route's source address wins over
// whatever the interface list happens to yield first, because a cloud host
// carries several routable addresses and only the default route's is
// reachable from outside.
package tests

import (
	"net"
	"testing"

	"github.com/Adelodunpeter25/console/apps/server-go/internal/cli/commands"
)

func TestSelectDialableAddressPrefersDefaultRoute(t *testing.T) {
	cases := []struct {
		name    string
		route   net.IP
		ifaces  []net.IP
		want    string
		comment string
	}{
		{
			name:   "default route beats a docker interface",
			route:  net.ParseIP("203.0.113.10"),
			ifaces: []net.IP{net.ParseIP("172.17.0.1"), net.ParseIP("10.0.0.4")},
			want:   "203.0.113.10",
		},
		{
			name:   "default route beats a vpc interface",
			route:  net.ParseIP("198.51.100.7"),
			ifaces: []net.IP{net.ParseIP("10.0.1.23")},
			want:   "198.51.100.7",
		},
		{
			name:   "no default route falls back to the first interface",
			route:  nil,
			ifaces: []net.IP{net.ParseIP("172.17.0.1"), net.ParseIP("10.0.0.4")},
			want:   "172.17.0.1",
		},
		{
			name:   "loopback only falls back to loopback",
			route:  nil,
			ifaces: nil,
			want:   "127.0.0.1",
		},
		{
			name:   "a loopback default route is ignored",
			route:  net.ParseIP("127.0.0.1"),
			ifaces: []net.IP{net.ParseIP("10.0.0.4")},
			want:   "10.0.0.4",
		},
		{
			name:   "an unspecified default route is ignored",
			route:  net.ParseIP("0.0.0.0"),
			ifaces: []net.IP{net.ParseIP("10.0.0.4")},
			want:   "10.0.0.4",
		},
		{
			name:   "ipv6 default route falls through to ipv4 interfaces",
			route:  net.ParseIP("2001:db8::1"),
			ifaces: []net.IP{net.ParseIP("10.0.0.4")},
			want:   "10.0.0.4",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := commands.SelectDialableAddress(tc.route, tc.ifaces)
			if got != tc.want {
				t.Errorf("SelectDialableAddress = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSelectDialableAddressIsUsableOnThisMachine checks the real lookup too,
// but only that it produces something dialable in shape — the address itself
// depends on the machine running the test.
func TestSelectDialableAddressIsUsableOnThisMachine(t *testing.T) {
	got := commands.SelectDialableAddress(nil, nil)
	if got == "" {
		t.Fatal("expected an address")
	}
	if net.ParseIP(got) == nil {
		t.Errorf("address %q is not an IP", got)
	}
}

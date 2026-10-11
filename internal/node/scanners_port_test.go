package node

import "testing"

// An event of a listener on a port range names one of its ports; the inbound's spec may
// be a port, a range or a list.
func TestPortIn(t *testing.T) {
	for _, c := range []struct {
		spec, port string
		in         bool
	}{
		{"443", "443", true}, {"443", "444", false},
		{"20000-20010", "20005", true}, {"20000-20010", "20011", false},
		{"443,20000-20010", "20000", true}, {"443, 8443", "8443", true},
		{"20000-20010", "x", false}, {"", "443", false},
	} {
		if got := portIn(c.spec, c.port); got != c.in {
			t.Errorf("portIn(%q, %q) = %v", c.spec, c.port, got)
		}
	}
}

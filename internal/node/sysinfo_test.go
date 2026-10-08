package node

import "testing"

// utime and stime are counted after the command name, which may hold spaces and parentheses.
func TestParseProcCPU(t *testing.T) {
	for name, tc := range map[string]struct {
		line string
		want uint64
		ok   bool
	}{
		"plain":  {"1662500 (mikan-node) S 1 1 1 0 -1 4194560 12 0 0 0 4100 2038 0 0 20 0 9 0 33 1317952000 17263", 6138, true},
		"spaces": {"7 (a (b) c) R 1 1 1 0 -1 0 0 0 0 0 10 5 0 0 20 0 1 0 1 1 1", 15, true},
		"short":  {"7 (x) R 1 2", 0, false},
		"broken": {"no name here", 0, false},
	} {
		got, err := parseProcCPU(tc.line)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("%s: got %d, %v", name, got, err)
		}
	}
}

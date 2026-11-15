package subs

import (
	"testing"

	"mikan/internal/panel/store/db"
)

func TestBalancedSkipsAllNodesTag(t *testing.T) {
	for _, c := range []struct {
		tags string
		want bool
	}{
		{"", true},
		{"[]", true},
		{`["семья"]`, true},
		{`["все-ноды"]`, false},
		{`["семья","Все-Ноды"]`, false},
		{`[" all-nodes "]`, false},
		{`["все-ноды-нет"]`, true},
	} {
		if got := balanced(db.User{Tags: c.tags}); got != c.want {
			t.Errorf("tags %s: balanced %v, want %v", c.tags, got, c.want)
		}
	}
}

package subs

import (
	"strings"

	"mikan/internal/panel/domain"
	"mikan/internal/panel/store/db"
)

// AllNodesTags: a client tagged with one of these is left out of load balancing: their
// subscription keeps every node of every balance group, as it was before the groups. For
// the admin and the people they look after, who switch servers by hand when one is blocked
// (Kinkan, 15.11.2026: two balance groups split clients over four nodes, the admin kept all).
var AllNodesTags = []string{"все-ноды", "all-nodes"}

// balanced reports whether the balance groups narrow this client's nodes.
func balanced(u db.User) bool {
	for _, t := range domain.DecodeTags(u.Tags) {
		for _, want := range AllNodesTags {
			if strings.EqualFold(strings.TrimSpace(t), want) {
				return false
			}
		}
	}
	return true
}

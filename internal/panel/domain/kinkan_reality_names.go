package domain

import (
	"context"
	"strings"

	"mikan/internal/panel/presets"
	"mikan/internal/panel/settings"
	"mikan/internal/panel/store/db"
	"mikan/internal/proto"
)

// kinkanForeignSNI refuses a REALITY name that belongs to the panel or to another node:
// the target on this node answers with its own certificate, so clients sending that name
// never finish the handshake. On 12.10.2026 both Finland nodes ended up with the panel's
// domain in server-names and VLESS stopped working there.
func (s *Inbounds) kinkanForeignSNI(ctx context.Context, node db.Node, t proto.Template) error {
	_, names := presets.Dest(t)
	if len(names) == 0 {
		return nil
	}
	set := settings.New(s.st.Q)
	panelDomain, err := set.String(ctx, settings.KeyDomain)
	if err != nil {
		return err
	}
	panelHost, err := set.String(ctx, settings.KeyPublicHost)
	if err != nil {
		return err
	}
	nodes, err := s.st.Q.ListNodes(ctx)
	if err != nil {
		return err
	}
	mine, theirs := map[string]bool{}, map[string]bool{}
	add := func(m map[string]bool, names ...string) {
		for _, n := range names {
			if n = strings.ToLower(strings.TrimSpace(n)); n != "" {
				m[n] = true
			}
		}
	}
	add(mine, node.Domain, node.PublicHost)
	if node.Address == "" { // the panel's own node
		add(mine, panelDomain, panelHost)
	} else {
		add(theirs, panelDomain, panelHost)
	}
	for _, n := range nodes {
		if n.ID != node.ID {
			add(theirs, n.Domain, n.PublicHost)
		}
	}
	for _, name := range names {
		if n := strings.ToLower(name); theirs[n] && !mine[n] {
			return &proto.Error{Code: "reality_sni_foreign", Field: "reality-config.server-names", Detail: name}
		}
	}
	return nil
}

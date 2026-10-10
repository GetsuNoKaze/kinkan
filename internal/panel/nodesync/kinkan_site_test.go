package nodesync

import (
	"archive/zip"
	"bytes"
	"context"
	"testing"

	"mikan/internal/nodeapi"
	"mikan/internal/site"
)

// siteNode is a node that takes sites: it reports the state's site missing until it was
// sent the archive.
type siteNode struct {
	*fakeNode
	has     map[string]bool
	puts    int
	applies int
}

func (n *siteNode) Apply(ctx context.Context, st nodeapi.DesiredState) (nodeapi.ApplyResult, error) {
	n.applies++
	res, err := n.fakeNode.Apply(ctx, st)
	if st.Site != nil && !n.has[st.Site.Hash] {
		res.SiteMissing = true
	}
	return res, err
}

func (n *siteNode) PutSite(_ context.Context, hash string, archive []byte) error {
	s, err := site.Unpack(archive)
	if err != nil || s.Hash != hash {
		return &nodeapi.Error{Code: "invalid_state", Message: "bad site"}
	}
	n.has[hash] = true
	n.puts++
	return nil
}

func syncSite(t *testing.T) *site.Site {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, data := range map[string]string{"index.html": "<title>Night</title>", "404.html": "x", "robots.txt": "x", "favicon.ico": "x"} {
		f, _ := w.Create(name)
		_, _ = f.Write([]byte(data))
	}
	_ = w.Close()
	s, err := site.Unpack(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// A node gets the site its state names once, and the state again; nothing more after that.
func TestNodeGetsItsSite(t *testing.T) {
	s, fake, st, _, _ := setup(t)
	ctx := context.Background()
	n := &siteNode{fakeNode: fake, has: map[string]bool{}}
	s.node = n
	kept, _, err := st.AddSite(ctx, "night", syncSite(t), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetNodeSite(ctx, LocalNode, kept.ID); err != nil {
		t.Fatal(err)
	}
	s.m.SlotsChanged() // what the API does after the change
	s.applyState(ctx)
	if n.puts != 1 || n.applies != 2 {
		t.Fatalf("puts=%d applies=%d, want the site sent once and the state applied twice", n.puts, n.applies)
	}
	last := fake.applied[len(fake.applied)-1]
	if last.Site == nil || last.Site.Hash != kept.Hash || last.Site.HTTPPort != SiteHTTPPort || last.Site.HTTPSPort != SiteHTTPSPort {
		t.Fatalf("applied site = %+v", last.Site)
	}
	s.applyState(ctx)
	if n.puts != 1 || n.applies != 2 {
		t.Fatalf("the same state again: puts=%d applies=%d", n.puts, n.applies)
	}

	// Taking the site away is a new state, without a site.
	if err := st.SetNodeSite(ctx, LocalNode, 0); err != nil {
		t.Fatal(err)
	}
	s.m.SlotsChanged()
	s.applyState(ctx)
	if last := fake.applied[len(fake.applied)-1]; last.Site != nil || n.puts != 1 {
		t.Fatalf("after taking the site away: %+v puts=%d", last.Site, n.puts)
	}
}

// A node that knows nothing of sites (Mikan's) takes the state as before; nothing is sent.
func TestNodeWithoutSitesIsLeftAlone(t *testing.T) {
	s, fake, st, _, _ := setup(t)
	ctx := context.Background()
	kept, _, err := st.AddSite(ctx, "night", syncSite(t), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.SetNodeSite(ctx, LocalNode, kept.ID); err != nil {
		t.Fatal(err)
	}
	s.applyState(ctx)
	if len(fake.applied) != 1 {
		t.Fatalf("applied %d states, want 1", len(fake.applied))
	}
}

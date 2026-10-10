package nodeapi

import (
	"encoding/json"
	"strings"
	"testing"
)

// The fork's embedded fields keep their JSON keys at the top level: nodes and panels of
// earlier Kinkan releases read them where they always were.
func TestKinkanFieldsKeepTheirPlace(t *testing.T) {
	for name, v := range map[string]any{
		`"site":{"hash":"h"`:  DesiredState{Site: &SiteState{Hash: "h"}},
		`"site_port":17443`:   ValidateRequest{SitePort: 17443},
		`"site":{"hash":"s"`:  Health{Site: &SiteStatus{Hash: "s"}},
		`"site_missing":true`: ApplyResult{SiteMissing: true},
	} {
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(raw), name) || strings.Contains(string(raw), "Kinkan") {
			t.Errorf("%T: %s", v, raw)
		}
	}
	var st DesiredState
	if err := json.Unmarshal([]byte(`{"revision":3,"site":{"hash":"h","http_port":17080}}`), &st); err != nil || st.Site == nil || st.Site.Hash != "h" || st.Revision != 3 {
		t.Fatalf("decoded %+v %v", st, err)
	}
}

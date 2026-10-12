package addons

import (
	"os"
	"testing"

	"mikan/internal/release"
)

// The marketplace's catalog of 11.10.2026, as published with its signature.
func TestKinkanCatalogIsCheckedWithMikansKey(t *testing.T) {
	data, err := os.ReadFile("testdata/kinkan-catalog.json")
	if err != nil {
		t.Fatal(err)
	}
	sig, err := os.ReadFile("testdata/kinkan-catalog.json.sig")
	if err != nil {
		t.Fatal(err)
	}
	pub, err := release.Key(kinkanCatalogKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = ParseCatalog(data, string(sig), pub, "0.5.0.5-tt.12"); err != nil {
		t.Fatalf("Mikan's catalog refused: %v", err)
	}
	if New("", "", "0.5.0.5-tt.12", nil, nil).pub.Equal(pub) == false {
		t.Fatal("the manager checks the catalog with another key")
	}
}

package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// addKinkanTexts adds the fork's errors.api texts (web/src/i18n/kinkan.<lang>.json, laid
// over Mikan's dictionary by the web) to Mikan's.
func addKinkanTexts(t *testing.T, root, lang string, api map[string]any) {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(root, "web", "src", "i18n", "kinkan."+lang+".json"))
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		Errors struct {
			API map[string]any `json:"api"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatalf("kinkan.%s.json: %v", lang, err)
	}
	for code, text := range f.Errors.API {
		if _, ok := api[code]; ok {
			t.Errorf("kinkan.%s.json: errors.api.%s is Mikan's too: the fork adds texts, it does not change them", lang, code)
		}
		api[code] = text
	}
}

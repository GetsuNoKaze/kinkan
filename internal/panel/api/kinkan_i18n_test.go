package api

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
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
		if prev, ok := api[code]; ok && !renamesMikan(prev, text) {
			t.Errorf("kinkan.%s.json: errors.api.%s is Mikan's too: the fork adds texts and changes only the product's name in them", lang, code)
		}
		api[code] = text
	}
}

// mikanName is Mikan's name for the product, not its config keys (mikan.client, mikan:)
// or CSS variables (--mikan-500).
var mikanName = regexp.MustCompile(`(^|[^-\w])([Mm])ikan(\.?$|[^.:\-\w]|\.[^A-Za-z])`)

// renamesMikan: the fork's text is Mikan's with the product called Kinkan, as Mikan's
// NOTICE.md asks of a modified version (GPL v3 section 7(c), 7(e)).
func renamesMikan(mikan any, kinkan any) bool {
	m, ok1 := mikan.(string)
	k, ok2 := kinkan.(string)
	if !ok1 || !ok2 || !mikanName.MatchString(m) {
		return false
	}
	return mikanName.ReplaceAllStringFunc(m, func(s string) string {
		return strings.Replace(strings.Replace(s, "Mikan", "Kinkan", 1), "mikan", "kinkan", 1)
	}) == k
}

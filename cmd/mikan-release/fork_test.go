package main

import (
	"strings"
	"testing"

	"mikan/internal/release"
)

func TestForkCheck(t *testing.T) {
	t.Setenv("GITHUB_REPOSITORY", release.Repo)
	if err := run([]string{"fork-check"}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"fork-check", "-repository", "someone/another-fork"}); err == nil {
		t.Fatal("accepted a release from a different repository")
	}
	t.Setenv("RELEASE_SIGNING_KEY", "")
	if err := run([]string{"fork-check", "-signing-key"}); err == nil {
		t.Fatal("accepted a missing signing key")
	}
	newFixture(t, changelog)
	if err := run([]string{"fork-check", "-signing-key"}); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("unrelated key: %v", err)
	}
}

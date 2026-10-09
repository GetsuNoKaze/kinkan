package main

import (
	"crypto/ed25519"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"mikan/internal/release"
)

// Stop a release before images are published if its repository or signing key is wrong.
func forkCheck(args []string) error {
	fs := flag.NewFlagSet("fork-check", flag.ContinueOnError)
	repo := fs.String("repository", os.Getenv("GITHUB_REPOSITORY"), "repository running the release")
	signing := fs.Bool("signing-key", false, "check RELEASE_SIGNING_KEY against the embedded public key")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 0 {
		return errors.New("fork-check: unexpected arguments")
	}
	if *repo != "" && !strings.EqualFold(*repo, release.Repo) {
		return fmt.Errorf("fork-check: repository %s differs from embedded %s", *repo, release.Repo)
	}
	if *signing {
		key, err := privateKey(os.Getenv("RELEASE_SIGNING_KEY"))
		if err != nil {
			return err
		}
		pub, err := release.Key(release.PublicKey)
		if err != nil {
			return err
		}
		if !pub.Equal(key.Public().(ed25519.PublicKey)) {
			return errors.New("fork-check: signing key differs from the public key trusted by this build")
		}
	}
	fmt.Printf("release repository: %s\n", release.Repo)
	return nil
}

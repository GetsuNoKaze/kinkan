# Kinkan releases (first release: 0.5.0.5-tt.1, as Mikan TT)

Repository: `GetsuNoKaze/kinkan`. Image: `ghcr.io/getsunokaze/kinkan`.
First candidate: `v0.5.0.5-tt.1`, based on upstream Mikan 0.5.0.4.
This file is a runbook; commands below have not been run on production nodes.

## Transition to stable Kinkan releases (tt.4 and later)

The first three fork releases were prereleases. Starting with the next release,
`vX-tt.N` is stable; `vX-tt.N-rc.N` remains beta. The existing tt.3 release is still
a prerelease. The steps below apply only after tt.4 has actually been published and
its signed manifest, installers and image have passed verification.

An installed tt.3 or earlier host command does not recognise `-tt.N` as stable.
Bootstrap once with the new release's installer script instead of running the old
`mikan update` command:

```sh
curl -fsSL https://github.com/GetsuNoKaze/kinkan/releases/download/v0.5.0.5-tt.4/install.sh | sudo bash
```

The script verifies the signed release index, manifest and installer checksum before
installing the host command. On an existing installation the new command performs an
update; it does not create a new panel. Back up the existing settings and database and
record the old image digest first. This also replaces the old signing key trusted by
tt.1/tt.2 with Kinkan's current key. After a successful update, verify `kinkan status`,
subscriptions and the other protocols; later stable releases use `kinkan update`.

Checked on the test node on 10.10.2026, both ways, with the database kept:

- **Mikan → Kinkan:** the one-line install above on a server running official Mikan
  0.5.0.4 updated it to 0.5.0.5-tt.4 (backup first, then the new image and command).
  A TrustTunnel inbound with `fallback` came back on its own; `smoke.sh` passed.
- **Kinkan → Mikan (rollback):**
  1. `mikan backup`, and note the current image (`grep MIKAN_IMAGE /opt/mikan/.env`).
  2. Download Mikan's installer for the target release and check its signed manifest
     with Mikan's key and the installer's sha256, as install.sh does.
  3. `./mikan-official update ghcr.io/miroshka000/mikan@sha256:<digest from its manifest>`.
     The database is kept: Kinkan adds no migrations of its own so far.
  4. **Replace the command too:** `install -m 755 mikan-official /usr/local/bin/mikan`,
     `rm -f /usr/local/bin/kinkan`, `mikan post-update`. Without it Kinkan's command
     stays, and since Kinkan's releases are stable it moves the server back to Kinkan on
     the next daily update check.
  5. Mikan's panel leaves a TrustTunnel inbound with `fallback` out of the node's state
     (`inbound left out of the node's state: trusttunnel: config_key (fallback)`); the
     other protocols keep working. Remove `fallback` from the template to get TT back on
     Mikan, with its `407` answers.

Once the fork adds database migrations of its own, step 3 also needs the backup from
before them restored.

## First-candidate procedure (historical)

1. Create the GitHub repository with no generated README or licence. Keep `upstream`
   pointing at Miroshka000/mikan; add `origin` pointing at GetsuNoKaze/kinkan.
   Push `main` only, not upstream tags. Upstream tags can trigger the release workflow.
2. Enable Actions. Add the fork's private PEM key as the `RELEASE_SIGNING_KEY` Actions
   secret. The matching public key is embedded in Go, Rust and install.sh. Keep a backup
   of the private key outside the checkout; it must never enter Git or a Docker context.
3. Run `ci` on main (push or workflow_dispatch). It checks the real web bundle, all Go
   tests with PostgreSQL and race detection, Rust tests, installer fault injection,
   dependency advisories and Docker builds for amd64 and arm64. It also runs real
   TrustTunnel client traffic and cover probes before and after a node restart in
   disposable containers. Run `tt check` too.
4. After all jobs pass, create and push the candidate tag. Release signing preflight
   must pass. The release workflow publishes versioned images, installers and signed
   manifests. This candidate is a prerelease; it does not move the image's `latest` tag.
   The default install.sh and installer select stable releases, so they cannot bootstrap
   this first candidate through the usual latest-release URL. Download the installer
   from this candidate's assets, verify its signed manifest and checksum, then use its
   `install --image ghcr.io/getsunokaze/kinkan@sha256:RELEASED_DIGEST` option on the
   disposable canary. Obtain RELEASED_DIGEST from the verified release manifest.
5. Set the package visibility to public if anonymous server pulls are required. Check
   that both architecture images and all release assets can be downloaded anonymously.
   Use the released digest for testing; never deploy a locally substituted test image.

The inherited upstream updater cannot migrate automatically to a new repository and
signing key. Keep the official deployment as the rollback target; switching only the
container image is insufficient to change the host installer or its update source.
Do not enable automatic updates until the fork's host installer has been installed and
its update command has been verified against the fork's signed release manifest.
Disable `mikan-update.timer` during migration; record and restore its previous state if
rolling back. Fresh installs also need the fork's installer, not the upstream README's URL.

## Isolated test node

Use a disposable Linux VM or a separate canary deployment before changing the four live
nodes. Save the existing image digest, /opt/mikan settings, panel database and Caddy
configuration. Do not run installer/test-install.sh or test-upgrade.sh on a real node:
these fault-injection scripts change /usr/local/bin/mikan and /opt/mikan.

1. Install the released fork installer and image. Verify the reported version, panel
   login, database migrations and node registration. Panel and node must both use the fork.
2. Append `deploy/tt/Caddyfile.fragment` to the existing Caddyfile. Validate the combined
   config inside the existing Caddy container before reloading it. The fragment assumes
   the existing /var/www/site to /srv/site mount. Confirm port 8080 is free and listens
   only on loopback. Keep existing HTTPS 8444 and REALITY certificates.
3. Check `curl --noproxy '*' --fail http://127.0.0.1:8080/` returns the cover page.
4. Add `fallback: 127.0.0.1:8080` to a TrustTunnel template in the panel. Use a free TCP
   port for the test inbound, open only that required port and use the node's valid
   domain certificate. Existing TCP 443 is occupied by VLESS XHTTP on all four nodes.
5. Run `bash scripts/tt/smoke.sh DOMAIN TRUSTTUNNEL_PORT`. The certificate must be valid;
   no `-k` is used. An HTTP/2-enabled curl is required. The CONNECT cover may legitimately
   answer 405; it must match the local cover response and must not advertise proxy auth.
6. Import the panel-generated subscription into a compatible TrustTunnel client.
   Verify authenticated traffic, DNS, the other enabled protocols, panel-generated
   credentials, listener restart, node restart and persistence of the fallback config.
7. Test a subsequent signed fork update, then restore the recorded official image,
   installer and settings. Verify panel, subscriptions and the other protocols after
   rollback. Restore the database backup if the tested update changed its schema.

Record the exact tested digest and results. A successful CI build alone does not mark
the authenticated-client, migration or rollback steps as passed.

## Release scope

TrustTunnel fallback and the panel's template support are included. An embedded cover
server, automatic fallback configuration, mikan-ensure, scanner counters and REALITY
target validation are future work. Caddy remains part of this first release.

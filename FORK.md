# mikan-tt

An unofficial fork of [mikan](https://github.com/Miroshka000/mikan). It is not affiliated with the mikan or mihomo authors, and everything not listed here is mikan as released. The fork follows mikan's releases: a workflow merges them and opens a pull request.

mikan's docs are kept as upstream reference; this file describes what the fork adds.
Use [RELEASE-TT.md](RELEASE-TT.md) for this fork's installation and release procedure.
The fork publishes to `GetsuNoKaze/mikan-tt`, with image `ghcr.io/getsunokaze/mikan-tt` and its own
release signing key. Official mikan's installer and updater do not install this fork.

## What is different

| | mikan | mikan-tt | State |
|---|---|---|---|
| TrustTunnel without credentials | answers `407`, which identifies the port as a proxy | requests, `CONNECT` included, go to a real site through `fallback` | done |
| TrustTunnel ALPN | none negotiated, browsers fall back to HTTP/1.1 | `h2, http/1.1` when `fallback` is set | done |
| Stalled clients on the fallback | | idle limit, 256 concurrent requests at most, cancelled on restart | done |
| `fallback` in a TrustTunnel template | rejected | accepted | done |
| Cover site | served by the panel only, on its port | served by every node on `127.0.0.1`, no Caddy needed | planned |
| Site for TrustTunnel | | `fallback` set automatically when the node has a domain | planned |
| REALITY targets | a wrong target goes unnoticed | checked: your certificate and your site behind your SNI | planned |
| Scanner check | | a command that runs the probes a scanner would and reports what gives the proxy away | planned |
| Scanner counter | | requests without credentials per day on TrustTunnel, in the panel | planned |
| Manual settings after an update | an update can reset them | `mikan-ensure` restores them and reports to Telegram | planned |

## TrustTunnel fallback

mihomo comes from `third_party/mihomo`: the version mikan's `go.mod` requires, with `patches/mihomo` applied. `scripts/tt/mihomo.sh` generates it, and `go.mod` replaces upstream mihomo with it. After a merge that moves mikan to another mihomo, run the script again (the sync workflow does).

The site the fallback points to has to answer plain HTTP, because TLS ends at mihomo. mikan nodes use host networking, so a site on the same server listening on `127.0.0.1:8080` works. In the TrustTunnel protocol template:

```yaml
type: trusttunnel
fallback: 127.0.0.1:8080
```

Every node with this template needs the fork's build, and so does the panel, which checks templates when they are saved. A node on mikan's build rejects the TrustTunnel inbound with a `fallback` key; its other inbounds keep working.

## Checks and sync

- `.github/workflows/tt-check.yml` runs `scripts/tt/check.sh` on every push and pull request: `third_party/mihomo` matches its patches, mikan builds, `go vet` passes, and the tests for the fork's changes pass. mikan's own `ci.yml` runs its full suite.
- `.github/workflows/tt-sync.yml` runs every 6 hours. It merges mikan's `main` into a `sync/upstream-<commit>` branch, regenerates `third_party/mihomo` if needed, runs the checks and opens a pull request. If the merge conflicts or a check fails, it opens an issue labelled `sync-failure`, or comments on the one already open, and the next run that passes closes it.
- Upstream tags are fetched under `upstream/`; they cannot overwrite this fork's release tags.

Repository settings the workflows need (release setup is in RELEASE-TT.md):

- Actions enabled for the fork, and under Settings, Actions, General: "Read and write permissions" and "Allow GitHub Actions to create and approve pull requests".
- Optional secret `SYNC_TOKEN`: a fine-grained token with Contents, Pull requests and Workflows write access to this repository. GitHub refuses pushes that change workflow files from the default token, so without it a sync that brings changes to mikan's workflows fails.
- Optional secrets `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` to get the failure and recovery messages in Telegram.

## Licence

GPL-3.0, like mikan and mihomo.

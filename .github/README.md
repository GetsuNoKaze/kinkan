<div align="center">

# 🍊 Kinkan

**A Mikan fork whose nodes answer probes like ordinary websites.**

[![Release](https://img.shields.io/github/v/release/GetsuNoKaze/kinkan?color=f0a020&label=release&style=flat-square)](https://github.com/GetsuNoKaze/kinkan/releases)
[![CI](https://img.shields.io/github/actions/workflow/status/GetsuNoKaze/kinkan/ci.yml?branch=main&label=ci&style=flat-square)](https://github.com/GetsuNoKaze/kinkan/actions/workflows/ci.yml)
[![License](https://img.shields.io/badge/license-GPL--3.0-2f9e6b?style=flat-square)](../LICENSE)

English · [Русский](README.ru.md)

</div>

Kinkan (金柑, kumquat — a small golden relative of the mikan) is an unofficial fork of
[Mikan](https://github.com/Miroshka000/mikan), the VPN panel on the mihomo core. Everything
Mikan does, Kinkan does: the panel, nodes, subscriptions, billing, the same protocols.
A workflow merges every Mikan release, checks it and opens a pull request.

What the fork adds is one idea: **a proxy is usually found not by its traffic but by
asking it**. A scanner connects without a password and looks at the answer; once one
protocol on a server gives itself away, the whole address tends to be blocked, with
every other protocol on it. Kinkan makes a node answer such questions the way an
ordinary website would.

## What is different

| | |
|---|---|
| **TrustTunnel with a cover site** | Requests without valid credentials are served by your own website instead of a proxy's error. HTTP/2 behaves like a regular web server's. |
| **A scanner check, built in** | The panel (Nodes → Check TrustTunnel) and `kinkan probe` look at a node the way a scanner would and say what gives it away. |
| **Safer settings** | The panel refuses settings that would expose the server, such as a cover site address that leads to the cloud metadata service. |
| **Its own signed releases** | Releases are signed with the fork's key; `kinkan update` and the daily update check install them. |

The full list is in [FORK.md](../FORK.md), the changes in [CHANGELOG-TT.md](../CHANGELOG-TT.md),
the plan in [ROADMAP.md](../ROADMAP.md): a quiet node mode, a cover site uploaded from the
panel and set up on every node, traffic per device.

## Install

On a fresh Debian or Ubuntu server, as root:

```sh
curl -fsSL https://github.com/GetsuNoKaze/kinkan/releases/latest/download/install.sh | sudo bash
```

The script checks the signed release index and the installer's checksum before it runs
anything. A node of an existing panel is installed with the key from the panel's Nodes page:

```sh
curl -fsSL https://github.com/GetsuNoKaze/kinkan/releases/latest/download/install.sh | sudo bash -s -- --join <key>
```

Afterwards the server has the `kinkan` command (`kinkan`, `kinkan status`, `kinkan update`);
`mikan` keeps working. Moving an existing Mikan server is a manual step:
[RELEASE-TT.md](../RELEASE-TT.md).

## Good to know

- Panel and nodes should all run Kinkan: Mikan's panel rejects the fork's settings.
- One protocol that gives itself away is enough to expose a server. Run the check after
  changing a node, and keep only the protocols you need open.
- Give every node its own cover site, or at least its own name: the same site on several
  servers ties them together.

## Credits and licence

Kinkan stands on [Mikan](https://github.com/Miroshka000/mikan) by Miroshka000 and on
[mihomo](https://github.com/MetaCubeX/mihomo) by MetaCubeX. It is not affiliated with
either: report problems with the fork here, not to them. GPL-3.0, like both.

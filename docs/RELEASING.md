# Release policy (how HyperDNS is versioned, published and updated)

This is the short operator-facing version of the README's *Versioning & release
policy* section, kept here so it sits next to the changelog it describes.

## SemVer

`MAJOR.MINOR.PATCH`.

- **MINOR** for new features — `2.8.0` → `2.9.0`.
- **PATCH** for backwards-compatible fixes — `2.8.0` → `2.8.1`.
- **MAJOR** for a breaking change (none so far; the whole `2.x` line is one
  contract).
- A pre-release tag (`v2.9.0-beta.1`, `v2.9.0-beta.2`, …) **precedes** the final
  `v2.9.0` of the same number. It is never after it.

## One source of truth

`version.json` at the repository root is the version every build embeds and the
version the dashboard's update check compares against. Bump it, commit, tag —
that is the whole release.

## Two channels

| Line | Tag shape | GitHub state | Who installs it |
|---|---|---|---|
| **Stable** | `v2.9.0` (no channel suffix) | Full release, takes *Latest* | everyone — the README one-liner and the in-dashboard updater |
| **Beta** | `v2.9.0-beta.1` | **Prerelease**, never *Latest* | anyone who opts in by hand (`install.sh`) |

## How the in-dashboard updater decides

1. It fetches `version.json` from the repository's **default branch** (`main`).
2. It compares that number with the version this build embeds — newer means an
   update is offered.
3. It resolves the release whose **tag matches that number exactly**; a `-beta`
   tag never satisfies a final target (and vice versa).
4. Since v2.8 the release must carry a **valid ed25519 signature** over
   `checksums.txt` (pinned in the binary, signed by the `RELEASE_SIGNING_KEY`
   Actions secret) **and** a matching SHA-256, and the client never honours a
   proxy.

So the update feed moves on exactly one event: **`main`'s `version.json` bumping
to a final number, followed by that tag.** It does not watch GitHub's *Latest*
badge, and beta tags do not move it.

## Where this repository stands

`main` carries `v2.2.0`, the release the project is known for, and it stays the
pinned stable line. The `2.3`–`2.6` betas that followed were published before
this policy existed; their prerelease flags have been corrected on GitHub, but
no number, tag, asset or link was changed. The stable line will be cut on `main`
when the work is releasable, and the updater feed starts moving that day.

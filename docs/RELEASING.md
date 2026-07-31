# Releasing

One release = one version across all four languages (lockstep). A single
`vX.Y.Z` tag triggers `.github/workflows/release.yml`, which publishes every
language in parallel; each job re-runs its language's full lint+test gate
first. The committed manifests keep version `0.0.0` — the publish version is
stamped from the tag, so there are no version-bump commits.

## Cutting a release

```sh
make release
```

Interactive: checks the working tree is clean, shows the latest released
version and HEAD, then offers to bump major/minor/patch (or force-recreate
the last tag on HEAD). On confirmation it creates and pushes a **tag pair**:

- `vX.Y.Z` — the release tag; triggers the workflow.
- `go/vX.Y.Z` — twin required by the Go module proxy for the module in the
  `go/` subdirectory. Triggers nothing (`v*` doesn't match `go/v*`).

Majors above `v1` are refused: they would require semantic import
versioning (`/v2` in the Go module path), which is not supported yet.

## What the workflow publishes

| Job | Registry | Version stamping | Auth |
|---|---|---|---|
| `go` | Go module proxy pulls the `go/vX.Y.Z` tag; job gates tests + creates the GitHub release | tag is the version | built-in `GITHUB_TOKEN` |
| `ts` | npmjs.org `@gopherex/schemapb` | `npm version --no-git-tag-version` | npm trusted publisher (OIDC) |
| `py` | PyPI `schemapb` | `uv version` before `uv build` | PyPI trusted publisher (OIDC) |
| `rust` | crates.io `schemapb` | `sed` on `Cargo.toml`, `cargo publish --allow-dirty` | crates.io trusted publisher (OIDC) |

## One-time trusted-publisher setup (per registry, no GitHub secrets)

Registries authenticate via trusted publishing (OIDC): each registry is
configured once to trust this repository + workflow file, GitHub mints a
short-lived identity token per run, and no long-lived secrets are stored.
All three entries point at repo `gopherex/schemapb`, workflow
**`release.yml`**:

- **npm** — npmjs.com → package `@gopherex/schemapb` → Settings → Trusted
  publisher → GitHub Actions. Do **not** set `NODE_AUTH_TOKEN` in the
  workflow — its presence makes npm fall back to the legacy token path.
  Provenance attestations are generated automatically. Requires npm ≥
  11.5.1 (bundled with Node 24).
- **PyPI** — pypi.org → project `schemapb` → Publishing → Add trusted
  publisher → GitHub. For the very first upload of a new project use a
  *pending publisher* on pypi.org. `pypa/gh-action-pypi-publish` uses OIDC
  automatically when no password is configured.
- **crates.io** — crates.io → crate `schemapb` → Settings → Trusted
  Publishing. The workflow exchanges the OIDC token via the official
  `rust-lang/crates-io-auth-action`, which mints a short-lived (~15 min)
  token and revokes it after the job. The very first `cargo publish` of a
  new crate must be done manually with a personal token — trusted
  publishing config requires an existing crate.

Go needs none of this: publishing is the tag itself.

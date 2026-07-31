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
| `ts` | npmjs.org `@gopherex/schemapb` | `npm version --no-git-tag-version` | `NPM_TOKEN` secret |
| `py` | PyPI `schemapb` | `uv version` before `uv build` | `PYPI_TOKEN` secret |
| `rust` | crates.io `schemapb` | `sed` on `Cargo.toml`, `cargo publish --allow-dirty` | `CARGO_REGISTRY_TOKEN` secret |

## One-time setup: three tokens → three GitHub secrets

Token auth was chosen over OIDC trusted publishing for setup simplicity
(releases are rare). Tokens can create first-time packages, so even the
very first release runs fully from CI — no manual publishes.

Create the tokens:

- **npm** (`NPM_TOKEN`) — npmjs.com: first create the `gopherex`
  organization (owns the `@gopherex` scope), then Access Tokens → Granular
  Access Token with read/write on packages of the `@gopherex` scope.
  ⚠ npm caps publish-token lifetime at **90 days**: when a release fails
  with 401/403, re-issue the token and update the secret.
- **PyPI** (`PYPI_TOKEN`) — pypi.org (account + mandatory 2FA) → Account
  settings → API tokens. For the first release the token must be
  account-scoped (project `schemapb` doesn't exist yet); after the first
  release you may replace it with a token scoped to the project. No expiry.
- **crates.io** (`CARGO_REGISTRY_TOKEN`) — crates.io (login via GitHub,
  **verified email required**) → Account Settings → API Tokens → scopes
  `publish-new` + `publish-update`. No expiry.

Store them in the GitHub repo: Settings → Secrets and variables → Actions →
New repository secret, names exactly `NPM_TOKEN`, `PYPI_TOKEN`,
`CARGO_REGISTRY_TOKEN`.

Go needs no token: publishing is the tag itself; the workflow only gates it
with tests and creates the GitHub release.

Rotation: any of the three can be revoked and re-issued at any time —
update the secret, re-run the failed job. If a language's publish failed
while others succeeded, re-running just that job is safe (registries refuse
duplicate versions; nothing is overwritten).

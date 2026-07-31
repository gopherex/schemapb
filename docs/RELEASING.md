# Releasing

Every language releases independently, driven by a tag with a language
prefix. The committed manifests keep version `0.0.0`; the publish version is
stamped from the tag inside the release workflow — the tag is the single
source of truth, no version-bump commits.

Registries authenticate via **trusted publishing** (OIDC): each registry is
configured once to trust this repository + workflow file, GitHub mints a
short-lived identity token per run, and no long-lived secrets are stored
anywhere.

| Language | Tag | Workflow | Registry | Auth |
|---|---|---|---|---|
| Go | `go/vX.Y.Z` | `release-go.yml` | Go module proxy (pulls the tag) | built-in `GITHUB_TOKEN` |
| TypeScript | `ts/vX.Y.Z` | `release-ts.yml` | npmjs.org (`@gopherex/schemapb`) | npm trusted publisher (OIDC) |
| Python | `py/vX.Y.Z` | `release-py.yml` | PyPI (`schemapb`) | PyPI trusted publisher (OIDC) |
| Rust | `rust/vX.Y.Z` | `release-rust.yml` | crates.io (`schemapb`) | crates.io trusted publisher (OIDC) |

The `go/` prefix is not a convention choice: the Go module lives in the `go/`
subdirectory, so the module proxy **requires** tags of the form `go/vX.Y.Z`.
The other prefixes mirror it for symmetry.

Every release workflow re-runs that language's full lint + test gate before
publishing; a red gate fails the release.

## Cutting a release

```sh
git tag ts/v0.1.0 && git push origin ts/v0.1.0
```

Versions are lockstep by policy: when the proto contract changes, tag all
four languages with the same version. A single-language fix may bump just
that language.

## One-time trusted-publisher setup (per registry, no GitHub secrets)

All three point at repo `gopherex/schemapb` and the matching workflow file:

- **npm** — npmjs.com → package `@gopherex/schemapb` → Settings → Trusted
  publisher → GitHub Actions, workflow `release-ts.yml`. Do **not** set
  `NODE_AUTH_TOKEN` in the workflow — its presence makes npm fall back to
  the legacy token path. Provenance attestations are generated
  automatically.
- **PyPI** — pypi.org → project `schemapb` → Publishing → Add trusted
  publisher → GitHub, workflow `release-py.yml`. (For the very first upload
  of a new project use a *pending publisher* on pypi.org.)
  `pypa/gh-action-pypi-publish` uses OIDC automatically when no password is
  configured.
- **crates.io** — crates.io → crate `schemapb` → Settings → Trusted
  Publishing, workflow `release-rust.yml`. The workflow exchanges the OIDC
  token via the official `rust-lang/crates-io-auth-action`, which mints a
  short-lived (~15 min) token and revokes it after the job. (The very first
  `cargo publish` of a new crate must be done manually with a personal
  token — trusted publishing config requires an existing crate.)

Go needs none of this: publishing is the tag itself; the workflow only
gates it with tests and creates a GitHub release
(`softprops/action-gh-release`).

## Version stamping mechanics

- TypeScript: `npm version --no-git-tag-version ${tag#ts/v}` before
  `npm publish` (npm ≥ 11.5.1, bundled with Node 24, required for OIDC).
- Python: `uv version ${tag#py/v}` before `uv build`.
- Rust: `sed` replaces the first `version = "0.0.0"` in `Cargo.toml`, then
  `cargo publish --allow-dirty`.

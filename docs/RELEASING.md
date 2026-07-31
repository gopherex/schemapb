# Releasing

Every language releases independently, driven by a tag with a language
prefix. The committed manifests keep version `0.0.0`; the publish version is
stamped from the tag inside the release workflow — the tag is the single
source of truth, no version-bump commits.

| Language | Tag | Workflow | Registry | Secret |
|---|---|---|---|---|
| Go | `go/vX.Y.Z` | `release-go.yml` | Go module proxy (pulls the tag) | — (built-in `GITHUB_TOKEN`) |
| TypeScript | `ts/vX.Y.Z` | `release-ts.yml` | npmjs.org (`@gopherex/schemapb`) | `NPM_TOKEN` |
| Python | `py/vX.Y.Z` | `release-py.yml` | PyPI (`schemapb`) | `PYPI_TOKEN` |
| Rust | `rust/vX.Y.Z` | `release-rust.yml` | crates.io (`schemapb`) | `CARGO_REGISTRY_TOKEN` |

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

## Secrets (GitHub repo → Settings → Secrets → Actions)

- `NPM_TOKEN` — npm **automation** token with publish rights to
  `@gopherex/schemapb`. The workflow publishes with `--provenance`
  (`id-token: write`).
- `PYPI_TOKEN` — PyPI API token scoped to the `schemapb` project
  (`pypi-…`). Passed to `pypa/gh-action-pypi-publish`.
- `CARGO_REGISTRY_TOKEN` — crates.io API token with publish scope.

Go needs no secret: publishing is the tag itself; the workflow only gates it
with tests and creates a GitHub release.

## Version stamping mechanics

- TypeScript: `npm version --no-git-tag-version ${tag#ts/v}` before
  `npm publish`.
- Python: `uv version ${tag#py/v}` before `uv build`.
- Rust: `sed` replaces the first `version = "0.0.0"` in `Cargo.toml`, then
  `cargo publish --allow-dirty`.

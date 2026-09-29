# Releasing

Go and TypeScript share a version. Python and Rust sources and local checks
remain available, but are excluded from CI and release workflows.

## Cutting a release

Run `make release` from a clean, committed checkout. Choose a new version;
never recreate a previously published version. Commit manifests remain `0.0.0`;
CI stamps the TypeScript version from the tag.

The release uses a tag pair on the same commit:

- `vX.Y.Z` triggers the release workflow.
- `go/vX.Y.Z` exposes the module in `go/` to Go consumers.

Push the branch and both tags together with `git push --atomic` when releasing
non-interactively. The workflow verifies the companion tag points to its commit.
Versions above v1 require Go semantic import versioning and are not supported.
Existing GitHub Releases also reserve their version even if a tag is missing.

## CI and publication

The reusable CI workflow runs proto lint, Go build/lint/tests and TypeScript
lint/typecheck/tests/build. Release publication depends on all three jobs passing.
The Go tag itself is already resolvable before the workflow completes, so run
these checks locally before pushing tags.

TypeScript publishes `@gopherex/schemapb` to `https://npm.pkg.github.com`, using
built-in `GITHUB_TOKEN` with `packages: write`. No npmjs/PyPI/crates.io token
is needed. The package repository metadata associates it with this repository.
The workflow creates the GitHub release after package publication and attaches
the built TypeScript tarball and SHA256SUMS. Published versions are immutable;
release a new version for changes rather than deleting packages or moving tags.

## Installation

Go:

```sh
go get github.com/gopherex/schemapb/go@v1.7.0
```

TypeScript: add this scope mapping to the consuming project's `.npmrc`:

```ini
@gopherex:registry=https://npm.pkg.github.com
```

Authenticate to GitHub Packages with a personal token with `read:packages`,
stored in user-level configuration or a CI secret, then install:

```sh
yarn add @gopherex/schemapb@1.7.0
```

GitHub Actions consumers need package read access for their `GITHUB_TOKEN`
or an authorized token with `read:packages`. Package visibility and access are
managed in GitHub Packages; do not commit credentials.

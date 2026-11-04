# Releasing

Releases are built by `.github/workflows/release.yml` with GoReleaser (`.goreleaser.yaml`) when a tag `v*` is pushed. Artifacts are named without a version (`rainbow-statusline_<os>_<arch>`) so `releases/latest/download/<name>` is a stable URL, and `checksums.txt` sits beside them.

## Check locally

```
goreleaser check
HOMEBREW_TAP_TOKEN=x SCOOP_BUCKET_TOKEN=x goreleaser release --snapshot --clean
RAINBOW_STATUSLINE_BASE_URL=file://$PWD/dist RAINBOW_STATUSLINE_DIR=$(mktemp -d) sh install.sh
```

## Homebrew and Scoop

Homebrew gets a plain formula, `brew install milczarekIT/tap/rainbow-statusline`. A cask would be wrong here: a cask download is quarantined, and an unsigned binary is then blocked by Gatekeeper, so the status line would silently never draw.

GoReleaser deprecates `brews` (and so `goreleaser check` exits non-zero with "DEPRECATED: brews"), but the version pinned in `release.yml` (v2.19.0) still renders a formula from it. The workflow does not run `goreleaser check`. When GoReleaser removes `brews` in a major version, either stay on v2.x or replace it with a formula generated in the workflow from a template kept in the repository.

Both packages publish to `milczarekIT/homebrew-tap` and `milczarekIT/scoop-bucket`, using the repository secrets `HOMEBREW_TAP_TOKEN` and `SCOOP_BUCKET_TOKEN`. While the repository variable `PUBLISH_PACKAGES` is not `true`, the workflow makes only the GitHub release.

`skip_upload: auto` additionally keeps pre-release tags (for example `v1.0.0-rc.1`) out of both repositories.

The workflow pins its actions by commit SHA and GoReleaser by exact version; update them deliberately.

## Pre-release

Nothing here has been run yet:

```
git tag v0.1.0-rc.1 && git push origin v0.1.0-rc.1
gh run watch --repo milczarekIT/rainbow-statusline
gh release download v0.1.0-rc.1 --repo milczarekIT/rainbow-statusline --dir /tmp/rs-rc
```

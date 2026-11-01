# Releasing

Releases are built by `.github/workflows/release.yml` with GoReleaser (`.goreleaser.yaml`) when a tag `v*` is pushed. Artifacts are named without a version (`rainbow-statusline_<os>_<arch>`) so `releases/latest/download/<name>` is a stable URL, and `checksums.txt` sits beside them.

## Check locally

```
goreleaser check
HOMEBREW_TAP_TOKEN=x SCOOP_BUCKET_TOKEN=x goreleaser release --snapshot --clean
RAINBOW_STATUSLINE_BASE_URL=file://$PWD/dist RAINBOW_STATUSLINE_DIR=$(mktemp -d) sh install.sh
```

## Homebrew and Scoop

GoReleaser no longer generates Homebrew formulae (`brews` is deprecated), so the tap gets a cask with a `binary` stanza and a quarantine-removal step. Install is `brew install --cask milczarekIT/tap/rainbow-statusline`.

Both packages publish to repositories that do not exist yet: `milczarekIT/homebrew-tap` and `milczarekIT/scoop-bucket`. Until the repository variable `PUBLISH_PACKAGES` is `true`, the workflow runs GoReleaser with `--skip=homebrew,scoop` and only the GitHub release is made. To enable them:

1. Create the two repositories (each with a README or an initial commit).
2. Create a token with `contents: write` on each, and store them as the repository secrets `HOMEBREW_TAP_TOKEN` and `SCOOP_BUCKET_TOKEN`.
3. Set the repository variable `PUBLISH_PACKAGES` to `true`.

`skip_upload: auto` additionally keeps pre-release tags (for example `v1.0.0-rc.1`) out of both repositories.

## Pre-release while the repository is private

For the owner to run later; nothing here has been run:

```
git tag v0.1.0-rc.1 && git push origin v0.1.0-rc.1
gh run watch --repo milczarekIT/rainbow-statusline
gh release download v0.1.0-rc.1 --repo milczarekIT/rainbow-statusline --dir /tmp/rs-rc
```

The anonymous paths (`curl`, `irm`, `brew`, `scoop`) can only be verified once the repository is public.

## Open items for publication

- Add a `LICENSE` file (and a license section in the README).
- Verify `install.ps1` and the Scoop manifest on Windows; `install.ps1` has not been run on any system yet.
- Verify the cask's `postflight_steps` on a real `brew install --cask`.

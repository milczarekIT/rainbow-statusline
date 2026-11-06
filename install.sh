#!/bin/sh
# Installs Rainbow Statusline for the current user (macOS, Linux). No sudo, no shell
# profile edits: if the install folder is not on PATH, the line to add is printed.
#
#   curl -fsSL https://raw.githubusercontent.com/milczarekIT/rainbow-statusline/main/install.sh | sh
#
# RAINBOW_STATUSLINE_VERSION  tag to install, e.g. v1.0.0 (default: latest)
# RAINBOW_STATUSLINE_DIR      install folder (default: ~/.local/bin)
# RAINBOW_STATUSLINE_BASE_URL download base holding the archive and checksums.txt
#                             (default: the GitHub release; mostly for testing)
set -eu

repo="milczarekIT/rainbow-statusline"
name="rainbow-statusline"

die() {
	printf '%s: %s\n' "$name install" "$*" >&2
	exit 1
}

# Everything runs inside main, called on the last line, so a download cut short
# in the middle of a "curl | sh" never executes half a script.
main() {
	case "$(uname -s)" in
	Darwin) os=darwin ;;
	Linux) os=linux ;;
	*) die "this script supports macOS and Linux; on Windows use install.ps1" ;;
	esac
	case "$(uname -m)" in
	arm64 | aarch64) arch=arm64 ;;
	x86_64 | amd64) arch=amd64 ;;
	*) die "unsupported CPU: $(uname -m)" ;;
	esac

	secure="--proto =https --tlsv1.2"
	if [ -n "${RAINBOW_STATUSLINE_BASE_URL:-}" ]; then
		base="$RAINBOW_STATUSLINE_BASE_URL"
		secure=""
	elif [ -n "${RAINBOW_STATUSLINE_VERSION:-}" ]; then
		base="https://github.com/$repo/releases/download/$RAINBOW_STATUSLINE_VERSION"
	else
		base="https://github.com/$repo/releases/latest/download"
	fi
	dir="${RAINBOW_STATUSLINE_DIR:-$HOME/.local/bin}"
	archive="${name}_${os}_${arch}.tar.gz"

	tmp="$(mktemp -d)"
	trap 'rm -rf "$tmp" "$dir/$name.new"' EXIT
	trap 'exit 1' INT TERM

	fetch() {
		# $secure is left unquoted on purpose: it is two options or nothing.
		# shellcheck disable=SC2086
		curl -fsSL $secure -o "$tmp/$1" "$base/$1" ||
			die "could not download $base/$1"
	}
	fetch "$archive"
	fetch checksums.txt

	want="$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")"
	[ -n "$want" ] || die "checksums.txt has no entry for $archive"
	if command -v sha256sum >/dev/null 2>&1; then
		got="$(sha256sum "$tmp/$archive" | awk '{ print $1 }')"
	else
		got="$(shasum -a 256 "$tmp/$archive" | awk '{ print $1 }')"
	fi
	[ "$want" = "$got" ] || die "checksum mismatch for $archive (expected $want, got $got); nothing was installed"

	tar -xzf "$tmp/$archive" -C "$tmp" "$name" || die "could not unpack $archive"
	mkdir -p "$dir"
	cp "$tmp/$name" "$dir/$name.new"
	chmod 755 "$dir/$name.new"
	# An unsigned download carries a quarantine flag that would stop it from running.
	xattr -d com.apple.quarantine "$dir/$name.new" 2>/dev/null || true
	mv -f "$dir/$name.new" "$dir/$name"

	printf 'Installed %s\n' "$dir/$name"
	next="$dir/$name"
	case ":$PATH:" in
	*":$dir:"*) next="$name" ;;
	*)
		# shellcheck disable=SC2016 # the $PATH in the printed line must stay literal
		printf '\n%s is not on your PATH. Add this line to your shell profile (~/.zshrc for zsh, ~/.bashrc for bash):\n  export PATH="%s:$PATH"\n' "$dir" "$dir"
		;;
	esac
	printf '\nNext: run  %s  to set it up.\n' "$next"
}

main "$@"

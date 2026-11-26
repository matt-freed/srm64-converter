#!/bin/sh
# Install (or update) the srm64 CLI from the latest GitHub release.
#
#   curl -fsSL https://raw.githubusercontent.com/matt-freed/srm64-converter/main/install.sh | sh
#
# or from a clone: ./install.sh
#
# Environment variables:
#   PREFIX         Install to $PREFIX/bin (default: $HOME/.local)
#   SRM64_VERSION  Install a specific release tag, e.g. v0.1.42 (default: latest)
#   SRM64_URL      Install from this tarball URL instead (for testing)
set -eu

REPO="matt-freed/srm64-converter"
PREFIX="${PREFIX:-$HOME/.local}"
BIN_DIR="$PREFIX/bin"

err() { echo "install.sh: $*" >&2; exit 1; }

case "$(uname -s)" in
	Darwin) PACKAGE="srm64-macos-universal" ;;
	Linux)
		case "$(uname -m)" in
			x86_64) PACKAGE="srm64-linux-x86_64" ;;
			aarch64 | arm64) PACKAGE="srm64-linux-arm64" ;;
			*) err "no prebuilt Linux binary for $(uname -m); build from source" ;;
		esac ;;
	*) err "unsupported OS $(uname -s); on Windows download the .zip from the releases page" ;;
esac

TMP=$(mktemp -d)
trap 'rm -rf "$TMP"' EXIT

if [ -n "${SRM64_URL:-}" ]; then
	echo "Downloading $SRM64_URL"
	curl -fSL --progress-bar "$SRM64_URL" -o "$TMP/srm64.tar.gz" || err "download failed"
else
	if [ -n "${SRM64_VERSION:-}" ]; then
		URL="https://github.com/$REPO/releases/download/$SRM64_VERSION/$PACKAGE.tar.gz"
	else
		URL="https://github.com/$REPO/releases/latest/download/$PACKAGE.tar.gz"
	fi
	echo "Downloading $URL"
	curl -fSL --progress-bar "$URL" -o "$TMP/srm64.tar.gz" || \
		err "download failed (does the release exist? see https://github.com/$REPO/releases)"
fi
tar -xzf "$TMP/srm64.tar.gz" -C "$TMP"
[ -f "$TMP/$PACKAGE/srm64" ] || err "archive does not contain $PACKAGE/srm64"

mkdir -p "$BIN_DIR"
# Install via a temp name + rename so a running copy is never half-written.
cp "$TMP/$PACKAGE/srm64" "$BIN_DIR/.srm64.new"
chmod 755 "$BIN_DIR/.srm64.new"
if [ "$(uname -s)" = "Darwin" ]; then
	# curl doesn't set the quarantine flag, but clear it in case something else did.
	xattr -d com.apple.quarantine "$BIN_DIR/.srm64.new" 2>/dev/null || true
fi
mv -f "$BIN_DIR/.srm64.new" "$BIN_DIR/srm64"

echo "Installed: $("$BIN_DIR/srm64" --version) -> $BIN_DIR/srm64"

case ":$PATH:" in
	*":$BIN_DIR:"*) ;;
	*)
		echo
		echo "$BIN_DIR is not on your PATH. Add it with:"
		case "${SHELL:-}" in
			*zsh)  echo "  echo 'export PATH=\"$BIN_DIR:\$PATH\"' >> ~/.zshrc && exec zsh" ;;
			*bash) echo "  echo 'export PATH=\"$BIN_DIR:\$PATH\"' >> ~/.bashrc && exec bash" ;;
			*)     echo "  export PATH=\"$BIN_DIR:\$PATH\"" ;;
		esac
		;;
esac

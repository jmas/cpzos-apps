#!/bin/bash
# Build the signed CPZ apps pacman repository from built packages.
#
# Inputs come from the environment:
#   GPG_PRIVATE_KEY   armored private signing key (GitHub secret)
#   GPG_PASSPHRASE    passphrase for the signing key (GitHub secret)
#   GPG_KEY_ID        signing key fingerprint (GitHub secret)
#   PKG_DIR           directory with built packages (default: dist/repo)
#   OUT_DIR           output directory (default: dist/signed)
#
# The output is uploaded as the assets of the GitHub release apps-stable:
#   https://github.com/jmas/cpzos-apps/releases/download/apps-stable/cpz-apps.db
set -euo pipefail
cd "$(dirname "$0")/.."

PKG_DIR="${PKG_DIR:-dist/repo}"
OUT_DIR="${OUT_DIR:-dist/signed}"

for var in GPG_PRIVATE_KEY GPG_PASSPHRASE GPG_KEY_ID; do
    [ -n "${!var:-}" ] || { echo "missing $var" >&2; exit 1; }
done

log() { printf '==> %s\n' "$*"; }

GNUPGHOME=$(mktemp -d)
export GNUPGHOME
trap 'rm -rf "$GNUPGHOME"' EXIT
chmod 700 "$GNUPGHOME"

printf '%s' "$GPG_PRIVATE_KEY" | gpg --batch --import >/dev/null 2>&1
FPR=$(gpg --list-secret-keys --with-colons "$GPG_KEY_ID" | \
    awk -F: '/^fpr:/ { print $10; exit }')
[ "$FPR" = "$GPG_KEY_ID" ] || {
    echo "signing key $GPG_KEY_ID is not available after import" >&2
    exit 1
}

sign() {
    gpg --batch --yes --pinentry-mode loopback \
        --passphrase "$GPG_PASSPHRASE" --local-user "$GPG_KEY_ID" \
        --detach-sign "$1"
}

log "preparing $OUT_DIR"
rm -rf "$OUT_DIR"
mkdir -p "$OUT_DIR"

shopt -s nullglob
packages=("$PKG_DIR"/*.pkg.tar.zst "$PKG_DIR"/*.pkg.tar.xz)
copied=()
for pkg in "${packages[@]}"; do
    cp "$pkg" "$OUT_DIR/"
    copied+=("$OUT_DIR/$(basename "$pkg")")
done
shopt -u nullglob
[ "${#copied[@]}" -gt 0 ] || { echo "no packages in $PKG_DIR" >&2; exit 1; }

log "signing packages"
for pkg in "${copied[@]}"; do
    sign "$pkg"
done

log "creating the repository database"
repo-add "$OUT_DIR/cpz-apps.db.tar.gz" "${copied[@]}"

# GitHub release assets cannot be symlinks; ship the database under its
# canonical name as well.
rm -f "$OUT_DIR/cpz-apps.db" "$OUT_DIR/cpz-apps.files"
cp "$OUT_DIR/cpz-apps.db.tar.gz" "$OUT_DIR/cpz-apps.db"
cp "$OUT_DIR/cpz-apps.files.tar.gz" "$OUT_DIR/cpz-apps.files"

log "signing the database"
sign "$OUT_DIR/cpz-apps.db"
sign "$OUT_DIR/cpz-apps.files"

log "repository ready: $OUT_DIR"
ls -l "$OUT_DIR"

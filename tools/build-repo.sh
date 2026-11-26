#!/bin/bash
# Build the CPZ app packages, the catalog and a local pacman repository.
#
# Runs inside an Arch environment (makepkg, repo-add, go, bsdtar). The result is
# dist/repo with cpz-apps.db.tar.gz and every package, suitable as a pacman
# Server for a test image.
set -euo pipefail
cd "$(dirname "$0")/.."

OUT=dist
REPO="$OUT/repo"
VERSION="${VERSION:-0.1.0}"

command -v makepkg >/dev/null || { echo "makepkg is required" >&2; exit 1; }
command -v repo-add >/dev/null || { echo "repo-add is required" >&2; exit 1; }
command -v go >/dev/null || { echo "go is required" >&2; exit 1; }
command -v bsdtar >/dev/null || { echo "bsdtar is required" >&2; exit 1; }

rm -rf "$OUT"
mkdir -p "$OUT/pkgs" "$REPO"

echo "==> building cpz-system-apps"
(
    cd packaging/cpz-system-apps
    makepkg -A --nodeps --nocheck -f --noconfirm
)
mv packaging/cpz-system-apps/cpz-system-apps-*.pkg.tar.* "$OUT/pkgs/"

echo "==> building platform dependencies"
(
    cd packaging/nethack
    makepkg -A --nodeps --nocheck -f --noconfirm
)
mv packaging/nethack/nethack-*.pkg.tar.* "$OUT/pkgs/"

echo "==> building mGBA"
(
    cd packaging/mgba
    makepkg -A --nodeps --nocheck -f --noconfirm
)
mv packaging/mgba/cpz-mgba-*.pkg.tar.* "$OUT/pkgs/"

echo "==> building application packages"
for pkgbuild in apps/*/PKGBUILD; do
    [ -f "$pkgbuild" ] || continue
    dir=$(dirname "$pkgbuild")
    echo "    $dir"
    (
        cd "$dir"
        makepkg -A --nodeps --nocheck -f --noconfirm
    )
    mv "$dir"/*.pkg.tar.* "$OUT/pkgs/"
done

echo "==> generating the catalog"
go run ./tools/build-catalog -packages "$OUT/pkgs" -out packaging/cpz-apps-catalog/apps.json

echo "==> checking catalog completeness"
go run ./tools/check-catalog -catalog packaging/cpz-apps-catalog/apps.json -apps apps

# Include the UTC time: a date-only version makes two catalog builds from the
# same day compare equal, so pacman -S skips the second and devices keep stale
# data forever.
CATALOG_VERSION="${CATALOG_VERSION:-$(date -u +%Y.%m.%d.%H%M%S)}"
echo "==> building cpz-apps-catalog $CATALOG_VERSION"
(
    cd packaging/cpz-apps-catalog
    CATALOG_VERSION="$CATALOG_VERSION" makepkg -A --nodeps --nocheck -f --noconfirm
)
mv packaging/cpz-apps-catalog/cpz-apps-catalog-*.pkg.tar.* "$OUT/pkgs/"

echo "==> repository"
cp "$OUT"/pkgs/*.pkg.tar.* "$REPO"/
( cd "$REPO" && repo-add cpz-apps.db.tar.gz ./*.pkg.tar.* >/dev/null )

echo "packages:"
ls -1 "$OUT"/pkgs
echo "repository: $REPO"

#!/usr/bin/env bash
# 打包发布包：packaging/build-package.sh <amd64|arm64> <版本号> <livems 二进制> <MediaServer 二进制> [输出目录]
# 产物：<输出目录>/streamswitch-linux-<arch>.tar.gz 及 .sha256
set -euo pipefail
export COPYFILE_DISABLE=1   # macOS 打包时不写入 ._* 扩展属性文件
ARCH=${1:?arch}; VERSION=${2:?version}; LIVEMS=${3:?livems}; ZLM=${4:?MediaServer}; OUT=${5:-dist}
ROOT=$(cd "$(dirname "$0")/.." && pwd)
WORK=$(mktemp -d "${TMPDIR:-/tmp}/sspkg.XXXXXX"); trap 'rm -rf "$WORK"' EXIT
PKG=$WORK/streamswitch
mkdir -p "$PKG"/{bin,templates,assets/fonts} "$OUT"
install -m 755 "$LIVEMS" "$PKG/bin/livems"
install -m 755 "$ZLM" "$PKG/bin/MediaServer"
cp -a "$ROOT/app/frontend/dist" "$PKG/web"
find "$PKG/web" \( -name '.DS_Store' -o -name '._*' \) -delete
cp "$ROOT/app/backend/assets/fonts/NotoSansSC-Regular.otf" "$PKG/assets/fonts/"
cp "$ROOT/packaging/zlm.ini" "$PKG/templates/zlm.ini"
install -m 755 "$ROOT/install.sh" "$PKG/install.sh"
cp "$ROOT/README.md" "$PKG/README.md"
echo "$VERSION" > "$PKG/VERSION"
echo "$ARCH" > "$PKG/ARCH"
NAME=streamswitch-linux-$ARCH.tar.gz
TAR_OPTS=()
tar --version 2>/dev/null | grep -q bsdtar && TAR_OPTS=(--no-xattrs --no-mac-metadata)   # macOS bsdtar：不写入扩展属性
tar "${TAR_OPTS[@]}" -C "$WORK" -czf "$OUT/$NAME" streamswitch
(cd "$OUT" && sha256sum "$NAME" > "$NAME.sha256" 2>/dev/null || shasum -a 256 "$NAME" > "$NAME.sha256")
echo "$OUT/$NAME ($(du -h "$OUT/$NAME" | cut -f1))"

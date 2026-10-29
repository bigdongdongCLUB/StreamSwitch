#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 在 ARM64 目标主机上从源码编译 ZLMediaKit（master），产物放 /root/zlm-build/
# 依赖：cmake gcc g++ libssl-dev（已由 apt 安装）
set -euo pipefail
SRC=/root/ZLMediaKit
OUT=/root/zlm-build
if [[ ! -d $SRC/.git ]]; then
  echo "==> 克隆 ZLMediaKit 源码"
  git clone --depth 1 --recurse-submodules --shallow-submodules https://github.com/ZLMediaKit/ZLMediaKit.git "$SRC" \
    || git clone --depth 1 --recurse-submodules --shallow-submodules https://gitee.com/xia-chu/ZLMediaKit.git "$SRC"
fi
cd "$SRC"
git log -1 --format='    commit %h %cd' --date=short
mkdir -p build && cd build
echo "==> cmake 配置"
cmake .. -DCMAKE_BUILD_TYPE=Release -DENABLE_TESTS=OFF -DENABLE_API=OFF -DENABLE_PLAYER=OFF \
         -DENABLE_WEBRTC=OFF -DENABLE_SRT=ON -DENABLE_OPENSSL=ON -DENABLE_FFMPEG=OFF > cmake.log 2>&1 || { tail -40 cmake.log; exit 1; }
echo "==> 编译 (make -j$(nproc))，约 5-15 分钟..."
make -j"$(nproc)" MediaServer > make.log 2>&1 || { tail -60 make.log; exit 1; }
mkdir -p "$OUT"
cp -a "$SRC/release/linux/Release/MediaServer" "$OUT/MediaServer"
[[ -d "$SRC/www" ]] && cp -a "$SRC/www" "$OUT/www" || true
"$OUT/MediaServer" -h 2>&1 | head -3 || true
ls -lh "$OUT/MediaServer"
echo ZLM_BUILD_DONE

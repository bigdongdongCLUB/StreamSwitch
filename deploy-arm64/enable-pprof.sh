#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# NanoPi 侧：安装新二进制并开启 pprof 诊断端口（仅 127.0.0.1:6060）
set -euo pipefail
APP=$APP_DIR
C=$APP/backend/configs/config.yaml
if ! grep -q '^  pprof_addr:' "$C"; then
  sed -i 's|^  static_dir: .*|&\n  pprof_addr: "127.0.0.1:6060"   # Go pprof 诊断（仅本机可访问）|' "$C"
fi
grep -n 'pprof_addr' "$C"
install -m 0755 /root/livems-pkg/livems-arm64 $APP/backend/bin/livems
systemctl restart livems
sleep 2
systemctl is-active livems
ss -ltn | grep -E ':(6060|8090) '
tail -n 3 $APP/backend/logs/livems.log | cut -c1-120

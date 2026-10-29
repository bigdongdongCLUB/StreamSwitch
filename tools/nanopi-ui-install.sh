#!/usr/bin/env bash
# 在测试主机上装一份（不注册用户）供浏览器检查初始化页面；并确认旧部署未受影响
. "$(dirname "$0")/env.sh"
set -uo pipefail
ssh -i "$SSH_KEY" "$SSH_HOST" 'bash -s' <<'REMOTE'
echo "--- 旧部署健康检查"
for s in livems zlm nginx; do printf '  %-8s %s\n' $s "$(systemctl is-active $s)"; done
curl -s -o /dev/null -w '  旧后台 :51880 -> HTTP %{http_code}\n' http://127.0.0.1:51880/health
echo "--- 安装 UI 测试实例（端口 53880）"
SS_YES=1 SS_HTTP_PORT=53880 SS_HTTPS_PORT=53443 SS_RTMP_PORT=3935 SS_RTSP_PORT=0 bash /root/sstest/streamswitch/install.sh install 2>&1 | grep -E '✔|✘|后台地址|首次打开'
curl -s http://127.0.0.1:53880/api/v1/setup/status; echo
REMOTE

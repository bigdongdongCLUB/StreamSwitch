#!/usr/bin/env bash
# =============================================================================
#  流媒体管理系统 - 一键部署/更新脚本（宝塔面板环境）
#
#  功能：编译后端 -> 部署二进制 -> 重载服务 -> 部署 Nginx 配置 -> 验证
#
#  用法：
#    bash deploy/scripts/deploy.sh           # 完整部署（编译+重启+nginx）
#    bash deploy/scripts/deploy.sh --no-nginx # 仅更新后端
# =============================================================================
set -euo pipefail

PROJ_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BACKEND_DIR="$PROJ_DIR/backend"
SITE_CONF="$(basename "$(grep -l server_name /www/server/panel/vhost/nginx/*.conf 2>/dev/null | head -1)" 2>/dev/null || echo '')"
NGINX_VHOST="/www/server/panel/vhost/nginx/streamswitch.conf"
SERVICE_NAME="livems"

WITH_NGINX=1
[[ "${1:-}" == "--no-nginx" ]] && WITH_NGINX=0

echo "==> [1/4] 编译后端"
cd "$BACKEND_DIR"
go build -trimpath -ldflags "-s -w" -o bin/livems.new ./cmd/server

echo "==> [2/4] 部署二进制并重启服务"
mv bin/livems.new bin/livems
systemctl restart "$SERVICE_NAME"
sleep 2
systemctl is-active --quiet "$SERVICE_NAME" || { echo "服务启动失败"; journalctl -u "$SERVICE_NAME" -n 20 --no-pager; exit 1; }

if [[ $WITH_NGINX -eq 1 ]]; then
  echo "==> [3/4] 部署 Nginx 站点配置"
  cp "$PROJ_DIR/deploy/nginx/streamswitch.conf" "$NGINX_VHOST"
  nginx -t
  nginx -s reload
else
  echo "==> [3/4] 跳过 Nginx 配置"
fi

echo "==> [4/4] 健康检查"
curl -fsS http://127.0.0.1:8090/health | grep -q '"code":0' && echo "后端健康检查通过"

echo ""
echo "部署完成。"
echo "  管理后台: https://streamswitch:51443/  (内网: http://streamswitch:51880/)"
echo "  默认账号: admin / admin123（首次登录请立即修改密码）"

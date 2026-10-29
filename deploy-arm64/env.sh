#!/usr/bin/env bash
# =============================================================================
#  deploy-arm64 脚本公共环境：从同目录的 .env.local 读取站点私有参数
#  （主机 IP、数据库密码、管理员密码、路由器 MAC 等不入库的信息）。
#
#  用法：在脚本开头  . "$(dirname "$0")/env.sh"
#  首次使用：cp .env.example .env.local 并填入实际值
#  部署到主机时把 env.sh 与 .env.local 一并放到脚本目录（/root/livems-pkg/）
# =============================================================================
ENV_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
if [[ -f "$ENV_DIR/.env.local" ]]; then
  set -a; . "$ENV_DIR/.env.local"; set +a
else
  echo "缺少 $ENV_DIR/.env.local，请先: cp $ENV_DIR/.env.example $ENV_DIR/.env.local 并填写" >&2
  exit 1
fi

# ---- 默认值（.env.local 未设置时生效）----
: "${HOST_IP:?.env.local 必须设置 HOST_IP}"
: "${SSH_USER:=root}"
: "${SSH_KEY:=$ENV_DIR/livems_key}"
: "${DB_NAME:=live_rextian_com}"
: "${DB_USER:=live_rextian_com}"
: "${DB_PASS:?.env.local 必须设置 DB_PASS}"
: "${ADMIN_USER:=admin}"
: "${ADMIN_PASS:?.env.local 必须设置 ADMIN_PASS}"
: "${ZLM_SECRET:?.env.local 必须设置 ZLM_SECRET}"
: "${APP_DIR:=/www/wwwroot/live.rextian.com}"
: "${ZLM_DIR:=/www/server/zlm}"
: "${LAN_CIDR:=192.168.1.0/24}"
: "${IFACE:=eth0}"
: "${ROUTER_IP:=}"
: "${ROUTER_MAC:=}"
: "${PROXY_GW_IP:=}"
: "${WAN_IP:=}"
: "${DDNS_DOMAIN:=}"
: "${WEB_PORT:=51880}"
: "${TLS_PORT:=51443}"

# ---- 派生 ----
SSH_HOST="$SSH_USER@$HOST_IP"
BASE_URL="http://$HOST_IP:$WEB_PORT"
# MySQL 客户端（在主机上执行时使用）
MYSQL="mysql -h127.0.0.1 -u$DB_USER -p$DB_PASS $DB_NAME"
# 登录 JSON（curl -d 用）
LOGIN_JSON="{\"username\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}"
export SSH_HOST BASE_URL MYSQL LOGIN_JSON

#!/usr/bin/env bash
# =============================================================================
#  流媒体管理系统 - 迁移还原脚本（在新服务器执行）
#
#  前置条件（新服务器为宝塔面板环境）：
#    1. 已安装宝塔面板，且装有 Nginx / MySQL / Redis
#    2. 已在宝塔创建 MySQL 数据库（或确认 root 可用）
#    3. 已在宝塔创建站点（域名随意，本脚本会覆盖其 conf）
#
#  用法：
#    bash deploy/migrate/restore.sh /root/livems-migrate-<时间戳>.tar.gz
#
#  选项：
#    --db-name xxx     数据库名（默认沿用源配置）
#    --db-user xxx     数据库账号（默认沿用源配置）
#    --db-pass xxx     数据库密码（默认沿用源配置，建议更换）
#    --skip-cert       不还原证书（自行在宝塔申请）
# =============================================================================
set -euo pipefail

PKG="${1:?用法: bash deploy/migrate/restore.sh <迁移包.tar.gz>}"
DB_OVERRIDE_NAME=""
DB_OVERRIDE_USER=""
DB_OVERRIDE_PASS=""
SKIP_CERT=0
shift
while [[ $# -gt 0 ]]; do
  case "$1" in
    --db-name) DB_OVERRIDE_NAME="$2"; shift 2;;
    --db-user) DB_OVERRIDE_USER="$2"; shift 2;;
    --db-pass) DB_OVERRIDE_PASS="$2"; shift 2;;
    --skip-cert) SKIP_CERT=1; shift;;
    *) echo "未知参数: $1"; exit 1;;
  esac
done

[[ "$(uname -m)" == "x86_64" ]] || { echo "警告: 当前架构 $(uname -m)，迁移包内二进制为 x86_64，无法运行"; exit 1; }
[[ -f "$PKG" ]] || { echo "迁移包不存在: $PKG"; exit 1; }

WORK="$(mktemp -d /tmp/livems-restore.XXXXXX)"
trap 'rm -rf "$WORK"' EXIT

echo "==> 解压迁移包"
tar -C "$WORK" -xzf "$PKG"
cat "$WORK/MIGRATION_INFO.txt" 2>/dev/null || true

APP_DIR="/www/wwwroot/streamswitch"
ZLM_DIR="/www/server/zlm"
BT_VHOST="/www/server/panel/vhost/nginx/streamswitch.conf"
BT_CERT_DIR="/www/server/panel/vhost/cert"

# ---- 1. 应用目录 ----
echo "==> [1/6] 部署应用目录 -> $APP_DIR"
mkdir -p "$APP_DIR"
rsync -a "$WORK/app/" "$APP_DIR/"
mkdir -p "$APP_DIR/backend/logs" "$APP_DIR/backend/data/hls_cache"

# ---- 2. 数据库 ----
CFG="$APP_DIR/backend/configs/config.yaml"
read_mysql_cfg() {
  awk -v key="$1" '
    /^mysql:/ { in_mysql=1; next }
    /^[a-z_]+:/ && !/^mysql:/ { in_mysql=0 }
    in_mysql && $0 ~ "^[[:space:]]+" key ":" {
      sub("^[[:space:]]*" key ":[[:space:]]*", "")
      gsub(/["'"'"']/, "")
      sub(/[[:space:]]*#.*$/, "")
      gsub(/[[:space:]]+$/, "")
      print; exit
    }
  ' "$CFG"
}
DB_NAME=${DB_OVERRIDE_NAME:-$(read_mysql_cfg database)}
DB_USER=${DB_OVERRIDE_USER:-$(read_mysql_cfg user)}
DB_PASS=${DB_OVERRIDE_PASS:-$(read_mysql_cfg password)}
DB_HOST=$(read_mysql_cfg host); DB_HOST=${DB_HOST:-127.0.0.1}
DB_PORT=$(read_mysql_cfg port); DB_PORT=${DB_PORT:-3306}

echo "==> [2/6] 导入数据库 $DB_NAME"
read -rp "请输入新服务器 MySQL root 密码（用于建库，直接回车跳过建库仅导入）: " -s ROOT_PASS; echo
if [[ -n "$ROOT_PASS" ]]; then
  mysql -h127.0.0.1 -uroot -p"$ROOT_PASS" -e "
    CREATE DATABASE IF NOT EXISTS \`$DB_NAME\` DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_unicode_ci;
    CREATE USER IF NOT EXISTS '$DB_USER'@'127.0.0.1' IDENTIFIED BY '$DB_PASS';
    CREATE USER IF NOT EXISTS '$DB_USER'@'localhost' IDENTIFIED BY '$DB_PASS';
    GRANT ALL PRIVILEGES ON \`$DB_NAME\`.* TO '$DB_USER'@'127.0.0.1';
    GRANT ALL PRIVILEGES ON \`$DB_NAME\`.* TO '$DB_USER'@'localhost';
    FLUSH PRIVILEGES;"
fi
mysql -h"$DB_HOST" -P"$DB_PORT" -u"$DB_USER" -p"$DB_PASS" "$DB_NAME" < "$WORK/db"/*.sql
echo "    数据库导入完成"

# ---- 3. ZLMediaKit ----
echo "==> [3/6] 部署 ZLMediaKit -> $ZLM_DIR"
mkdir -p "$ZLM_DIR/bin" "$ZLM_DIR/conf" "$ZLM_DIR/www" "$ZLM_DIR/log"
cp -a "$WORK/zlm/bin/MediaServer" "$ZLM_DIR/bin/"
cp -a "$WORK/zlm/conf/config.ini" "$ZLM_DIR/conf/config.ini"
chmod +x "$ZLM_DIR/bin/MediaServer"

# ---- 4. systemd ----
echo "==> [4/6] 安装 systemd 服务"
cp -a "$WORK/systemd/zlm.service" /etc/systemd/system/
cp -a "$WORK/systemd/livems.service" /etc/systemd/system/
systemctl daemon-reload
systemctl enable zlm livems

# ---- 5. Nginx 站点配置 ----
echo "==> [5/6] 部署 Nginx 站点配置"
if [[ -f "$APP_DIR/deploy/nginx/streamswitch.conf" ]]; then
  cp -a "$APP_DIR/deploy/nginx/streamswitch.conf" "$BT_VHOST"
  nginx -t
fi

# ---- 6. SSL 证书 ----
if [[ $SKIP_CERT -eq 0 && -d "$WORK/certs/default" ]]; then
  echo "==> [6/6] 还原 SSL 证书"
  mkdir -p "$BT_CERT_DIR"
  cp -a "$WORK/certs/default" "$BT_CERT_DIR/"
else
  echo "==> [6/6] 跳过证书还原（请在宝塔面板重新申请）"
fi

echo ""
echo "==> 启动服务"
systemctl restart zlm
sleep 3
systemctl restart livems
sleep 2
nginx -s reload 2>/dev/null || true

echo ""
echo "==> 健康检查"
systemctl is-active --quiet zlm && echo "  ZLM:    运行中" || echo "  ZLM:    启动失败（journalctl -u zlm -n 30）"
systemctl is-active --quiet livems && echo "  livems: 运行中" || echo "  livems: 启动失败（journalctl -u livems -n 30）"
curl -fsS --max-time 5 http://127.0.0.1:8090/health | grep -q '"code":0' && echo "  API:    正常" || echo "  API:    异常"

echo ""
echo "================================================================"
echo " 迁移完成。后续事项："
echo "  1. 后台地址: http://<服务器IP>:51880/ 或 https://<域名>:51443/"
echo "  2. DNS 切换: 把域名 A 记录指向新服务器 IP"
echo "  3. 防火墙/映射: 放行 51880 / 51443 / 1935"
echo "  4. 若更换了域名: 后台「域名配置」修改推流/播放域名"
echo "  5. 若未还原证书: 宝塔面板为站点申请 SSL 证书"
echo "================================================================"

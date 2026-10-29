#!/usr/bin/env bash
# =============================================================================
#  流媒体管理系统 - 迁移打包脚本（在源服务器执行）
#
#  用法：
#    bash deploy/migrate/pack.sh                 # 完整打包（含素材库）
#    bash deploy/migrate/pack.sh --no-materials  # 不打包制播素材（素材可后续 rsync）
#
#  产物：项目根目录上一级生成 livems-migrate-<时间戳>.tar.gz
#
#  包含：应用目录（后端/前端/配置/工具/字体/素材）、MySQL 全库 dump、
#        ZLMediaKit（二进制+配置）、systemd 单元、SSL 证书、迁移信息清单
# =============================================================================
set -euo pipefail

PROJ_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
TIMESTAMP="$(date +%Y%m%d-%H%M%S)"
OUT="${PROJ_DIR}/../livems-migrate-${TIMESTAMP}.tar.gz"
WORK="$(mktemp -d /tmp/livems-migrate.XXXXXX)"
WITH_MATERIALS=1
[[ "${1:-}" == "--no-materials" ]] && WITH_MATERIALS=0

# 从 config.yaml 读取数据库连接（限定 mysql: 段内解析，避免与 server/redis 段的同名键冲突）
CFG="$PROJ_DIR/backend/configs/config.yaml"
[[ -f "$CFG" ]] || { echo "找不到配置文件: $CFG"; exit 1; }
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
DB_HOST=$(read_mysql_cfg host)
DB_PORT=$(read_mysql_cfg port)
DB_USER=$(read_mysql_cfg user)
DB_PASS=$(read_mysql_cfg password)
DB_NAME=$(read_mysql_cfg database)
DB_PORT=${DB_PORT:-3306}
[[ -z "$DB_HOST" || -z "$DB_USER" || -z "$DB_NAME" ]] && { echo "从 config.yaml 读取数据库配置失败"; exit 1; }

echo "==> [1/6] 收集应用目录"
mkdir -p "$WORK/app"
rsync -a \
  --exclude 'backend/logs/' \
  --exclude 'backend/data/hls_cache/' \
  --exclude 'backend/bin/livems.new' \
  --exclude 'backend/bin/*.old' \
  $([[ $WITH_MATERIALS -eq 0 ]] && echo "--exclude 'backend/data/studio/'" || true) \
  "$PROJ_DIR/backend" "$PROJ_DIR/frontend" "$PROJ_DIR/deploy" "$PROJ_DIR/sql" "$PROJ_DIR/docs" \
  "$WORK/app/"

echo "==> [2/6] 导出数据库 $DB_NAME"
mkdir -p "$WORK/db"
mysqldump -h"$DB_HOST" -P"$DB_PORT" -u"$DB_USER" -p"$DB_PASS" \
  --single-transaction --routines --triggers --default-character-set=utf8mb4 \
  "$DB_NAME" > "$WORK/db/${DB_NAME}.sql"
echo "    数据库导出完成: $(du -sh "$WORK/db/${DB_NAME}.sql" | cut -f1)"

echo "==> [3/6] 收集 ZLMediaKit（二进制 + 配置）"
mkdir -p "$WORK/zlm/bin" "$WORK/zlm/conf"
cp -a /www/server/zlm/bin/MediaServer "$WORK/zlm/bin/"
cp -a /www/server/zlm/conf/config.ini "$WORK/zlm/conf/"

echo "==> [4/6] 收集 systemd 单元"
mkdir -p "$WORK/systemd"
cp -a /etc/systemd/system/livems.service "$WORK/systemd/" 2>/dev/null || cp -a "$PROJ_DIR/deploy/systemd/livems.service" "$WORK/systemd/"
cp -a /etc/systemd/system/zlm.service "$WORK/systemd/"

echo "==> [5/6] 收集 SSL 证书"
mkdir -p "$WORK/certs"
if [[ -d /www/server/panel/vhost/cert/default ]]; then
  cp -a /www/server/panel/vhost/cert/default "$WORK/certs/"
else
  echo "    警告: 宝塔证书目录不存在，跳过（新服务器需重新申请证书）"
fi

echo "==> [6/6] 生成迁移信息清单并打包"
{
  echo "打包时间: $(date '+%F %T')"
  echo "源主机: $(hostname) ($(uname -m))"
  echo "livems: $(cd "$PROJ_DIR/backend" && ./bin/livems -version 2>/dev/null | head -1 || echo unknown)"
  echo "ZLM: $(/www/server/zlm/bin/MediaServer -v 2>/dev/null | grep -E '编译日期|git' | head -2 | tr '\n' ' ')"
  echo "数据库: $DB_NAME @ $DB_HOST:$DB_PORT"
  echo "素材打包: $([[ $WITH_MATERIALS -eq 1 ]] && echo 是 || echo 否)"
  echo "数据库行数:"
  mysql -h"$DB_HOST" -P"$DB_PORT" -u"$DB_USER" -p"$DB_PASS" "$DB_NAME" -N -e \
    "SELECT CONCAT('  ', table_name, ': ', table_rows) FROM information_schema.tables WHERE table_schema='$DB_NAME' ORDER BY table_name;" 2>/dev/null || true
} > "$WORK/MIGRATION_INFO.txt"

tar -C "$WORK" -czf "$OUT" .
rm -rf "$WORK"
echo ""
echo "打包完成: $OUT ($(du -sh "$OUT" | cut -f1))"
echo "传输到新服务器: scp $OUT root@<新服务器>:/root/"
echo "新服务器还原:   bash deploy/migrate/restore.sh /root/$(basename "$OUT")"

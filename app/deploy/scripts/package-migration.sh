#!/usr/bin/env bash
# =============================================================================
#  生成迁移包（一站式打包，交给运维直接传到新服务器）
#
#  产出：
#    /www/wwwroot/livems-migrate-<日期>.tar.gz
#      ├── project/          完整项目源码（含 Docker 配置、宝塔包、迁移数据）
#      ├── images/           Docker 镜像（可选，--with-images）
#      └── README.txt        快速上手说明
#
#  用法：
#    bash deploy/scripts/package-migration.sh [--with-images]
# =============================================================================
set -euo pipefail

PROJ_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
WITH_IMAGES=0
[[ "${1:-}" == "--with-images" ]] && WITH_IMAGES=1

DATE="$(date +%Y%m%d)"
STAGE="/tmp/livems-migrate-$DATE"
PKG="/www/wwwroot/livems-migrate-$DATE.tar.gz"

echo "==> 清理旧的暂存目录"
rm -rf "$STAGE"
STAGE="/www/wwwroot/.livems-migrate-stage-$DATE"
mkdir -p "$STAGE/project"

echo "==> 复制项目文件（排除运行时与构建产物）"
rsync -a --no-owner --no-group \
    --exclude='backend/logs/*' \
    --exclude='backend/data/hls_cache/*' \
    --exclude='deploy/docker/images/' \
    --exclude='deploy/baota/dist/' \
    --exclude='.zhanlu/' \
    --exclude='*.log' \
    "$PROJ_DIR/" "$STAGE/project/"

echo "==> 打包 Docker 编排配置与迁移数据"
mkdir -p "$STAGE/project/deploy/docker/data"
if [[ -f "$PROJ_DIR/deploy/docker/data/migration.sql" ]]; then
    cp -a "$PROJ_DIR/deploy/docker/data/." "$STAGE/project/deploy/docker/data/"
fi

# 敏感文件不进包：.env 用模板，运行时文件单独放
rm -f "$STAGE/project/deploy/docker/.env"

if [[ $WITH_IMAGES -eq 1 ]]; then
    echo "==> 导出 Docker 镜像"
    bash "$PROJ_DIR/deploy/docker/scripts/export-images.sh" >/dev/null
    mkdir -p "$STAGE/images"
    cp -a "$PROJ_DIR/deploy/docker/images/." "$STAGE/images/"
fi

cat > "$STAGE/README.txt" <<EOF
流媒体管理系统 - 迁移包
生成时间: $(date '+%Y-%m-%d %H:%M:%S %Z')

【内容】
  project/          完整项目源码
    deploy/docker/          Docker 编排（推荐方案）
    deploy/baota/           宝塔 Go 项目方案
    deploy/MIGRATION.md     迁移总览 + 检查清单
    sql/schema.sql          数据库表结构
    backend/                Go 后端源码
    frontend/dist/          前端产物
$( [[ $WITH_IMAGES -eq 1 ]] && echo "  images/           Docker 镜像（离线导入用）" )

【快速上手】
  1. 解压：tar xzf livems-migrate-$DATE.tar.gz
  2. 传到新服务器后进 project/ 目录
  3. 选择方案：

  方案 A - Docker（推荐）：
     cd project/deploy/docker
     cp .env.example .env && vi .env     # 改密码、域名、JWT_SECRET
     docker-compose up -d
     bash scripts/restore-runtime.sh
     docker-compose restart livems
     详见 project/deploy/docker/README.md

  方案 B - 宝塔 Go 项目：
     详见 project/deploy/baota/README.md

【必改配置】
  - JWT_SECRET        （openssl rand -base64 48）
  - MYSQL_PASSWORD    数据库密码
  - ZLM_SECRET        必须与 zlm/config.ini 的 api.secret 一致
  - SERVER_NAME       对外域名

【防火墙】
  放行 51880/tcp (HTTP)、51443/tcp (HTTPS)、1935/tcp (RTMP)
  严禁开放 3306 / 6379 / 8080 / 8090

【文档】
  project/deploy/MIGRATION.md          迁移总览、清单、检查清单
  project/deploy/docker/README.md      Docker 完整文档
  project/deploy/baota/README.md       宝塔 Go 项目完整文档
  project/docs/deploy.md               原始架构与部署文档
EOF

echo "==> 打包"
tar czf "$PKG" -C "$STAGE" .

echo ""
echo "==> 完成"
ls -lh "$PKG"
du -sh "$STAGE"
rm -rf "$STAGE"

echo ""
echo "传到新服务器："
echo "    scp $PKG root@<新服务器>:/root/"
echo "新服务器解压："
echo "    tar xzf livems-migrate-$DATE.tar.gz -C /root/"

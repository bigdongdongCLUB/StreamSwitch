#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# =============================================================================
#  流媒体管理系统 - ARM64 / 原生 Ubuntu 24.04 部署脚本（在目标主机执行）
#
#  与 deploy/migrate/restore.sh 的差异：
#    - 目标为 aarch64，不使用包内 x86_64 二进制：
#        livems    -> 使用交叉编译产物 /root/livems-pkg/livems-arm64
#        ffmpeg    -> 使用 apt 安装的系统 ffmpeg（含 drawtext/overlay）
#        ZLM       -> 从源码编译（单独由 build-zlm.sh 完成，本脚本只做安装）
#    - 非宝塔环境：Nginx 配置放 /etc/nginx/conf.d/，证书目录沿用宝塔路径以兼容后台证书管理
#    - MySQL 为 Ubuntu 自带 8.0，root 通过 auth_socket 免密
#
#  用法： bash deploy-arm64.sh [步骤...]   不带参数=全部；可选: app db zlm systemd nginx start check
# =============================================================================
set -euo pipefail

PKG=/root/livems-pkg
CERT_DIR=/www/server/panel/vhost/cert
NGX_CONF=/etc/nginx/conf.d/live.rextian.com.conf

STEPS="${*:-app db zlm systemd nginx start check}"
has() { [[ " $STEPS " == *" $1 "* ]]; }

# ---- 1. 应用目录 ----
if has app; then
  echo "==> [app] 部署应用目录 -> $APP_DIR"
  mkdir -p "$APP_DIR"
  rsync -a --exclude 'backend/bin' "$PKG/app/" "$APP_DIR/"
  mkdir -p "$APP_DIR/backend/bin/tools" "$APP_DIR/backend/logs" "$APP_DIR/backend/data/hls_cache" "$APP_DIR/backend/data/hls_key"
  install -m 0755 "$PKG/livems-arm64" "$APP_DIR/backend/bin/livems"
  # 配置文件：仓库不含 config.yaml（有密钥），首次部署由模板生成并填入 .env.local 的值
  if [[ ! -f "$APP_DIR/backend/configs/config.yaml" ]]; then
    cp "$APP_DIR/backend/configs/config.example.yaml" "$APP_DIR/backend/configs/config.yaml"
    sed -i -e "s#^\(  user: \).*#\1$DB_USER#" -e "s#^\(  database: \).*#\1$DB_NAME#" \
           -e "0,/^  password: /s#^\(  password: \).*#\1\"$DB_PASS\"   \# MySQL#" \
           -e "s#^\(  secret: \)\"CHANGE_ME_zlm_api_secret\"#\1\"$ZLM_SECRET\"#" \
           -e "s#^\(  secret: \)\"CHANGE_ME_livems_jwt_secret_please_change_it\"#\1\"$(head -c 24 /dev/urandom | base64 | tr -d '=+/')\"#" \
           "$APP_DIR/backend/configs/config.yaml"
    echo "    已从模板生成 config.yaml（数据库/ZLM/JWT 密钥已填入）"
  fi
  ln -sfn /usr/bin/ffmpeg  "$APP_DIR/backend/bin/tools/ffmpeg"
  ln -sfn /usr/bin/ffprobe "$APP_DIR/backend/bin/tools/ffprobe"
  # 配置文件：Nginx vhost 路径改为原生 nginx 位置
  sed -i "s#vhost_conf: .*#vhost_conf: \"$NGX_CONF\"#" "$APP_DIR/backend/configs/config.yaml"
  "$APP_DIR/backend/bin/livems" -version
  "$APP_DIR/backend/bin/tools/ffmpeg" -hide_banner -version | head -1
  "$APP_DIR/backend/bin/tools/ffmpeg" -hide_banner -filters 2>/dev/null | grep -q ' drawtext ' && echo "    ffmpeg drawtext: OK" || echo "    警告: ffmpeg 缺少 drawtext 滤镜"
fi

# ---- 2. 数据库 ----
if has db; then
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
  DB_NAME=$(read_mysql_cfg database); DB_USER=$(read_mysql_cfg user); DB_PASS=$(read_mysql_cfg password)
  echo "==> [db] 建库建账号并导入 $DB_NAME"
  systemctl enable --now mysql redis-server
  mysql -uroot -e "
    CREATE DATABASE IF NOT EXISTS \`$DB_NAME\` DEFAULT CHARSET utf8mb4 COLLATE utf8mb4_unicode_ci;
    CREATE USER IF NOT EXISTS '$DB_USER'@'127.0.0.1' IDENTIFIED BY '$DB_PASS';
    CREATE USER IF NOT EXISTS '$DB_USER'@'localhost' IDENTIFIED BY '$DB_PASS';
    GRANT ALL PRIVILEGES ON \`$DB_NAME\`.* TO '$DB_USER'@'127.0.0.1';
    GRANT ALL PRIVILEGES ON \`$DB_NAME\`.* TO '$DB_USER'@'localhost';
    FLUSH PRIVILEGES;"
  mysql -h127.0.0.1 -u"$DB_USER" -p"$DB_PASS" "$DB_NAME" < "$PKG"/db/*.sql
  # 新主机 IP：把「内网直连」播放域名与推流主机指向本机
  mysql -h127.0.0.1 -u"$DB_USER" -p"$DB_PASS" "$DB_NAME" -e "
    UPDATE play_domain SET host='$HOST_IP', port=51880, \`schema\`='http', remark='内网 IP 直连（新主机）', updated_at=UNIX_TIMESTAMP() WHERE id=6;
    UPDATE play_domain SET is_default=IF(id=6,1,0);
    UPDATE sys_config SET config_val=JSON_SET(config_val,'$.push_host','$HOST_IP') WHERE config_key='ingress';"
  echo "    行数校验:"
  for t in stream play_domain play_token sys_user sys_permission studio_material studio_task; do
    printf '      %-18s %s\n' "$t" "$(mysql -h127.0.0.1 -u"$DB_USER" -p"$DB_PASS" "$DB_NAME" -N -e "SELECT COUNT(*) FROM $t" 2>/dev/null)"
  done
fi

# ---- 3. ZLMediaKit ----
if has zlm; then
  echo "==> [zlm] 安装 ZLMediaKit -> $ZLM_DIR"
  [[ -x /root/zlm-build/MediaServer ]] || { echo "未找到 /root/zlm-build/MediaServer，请先执行 build-zlm.sh"; exit 1; }
  mkdir -p "$ZLM_DIR/bin/www" "$ZLM_DIR/conf" "$ZLM_DIR/log"
  install -m 0755 /root/zlm-build/MediaServer "$ZLM_DIR/bin/MediaServer"
  cp -a "$PKG/zlm/conf/config.ini" "$ZLM_DIR/conf/config.ini"
  sed -i "s/^secret=.*/secret=$ZLM_SECRET/" "$ZLM_DIR/conf/config.ini"   # 仓库内为占位值
  # ZLM 自带 www 资源（crossdomain.xml 等）如编译目录有则一并复制
  [[ -d /root/zlm-build/www ]] && cp -an /root/zlm-build/www/. "$ZLM_DIR/bin/www/" || true
fi

# ---- 4. systemd ----
if has systemd; then
  echo "==> [systemd] 安装服务单元"
  cp -a "$PKG/systemd/zlm.service" "$PKG/systemd/livems.service" /etc/systemd/system/
  systemctl daemon-reload
  systemctl enable zlm livems
fi

# ---- 5. Nginx + 证书 ----
if has nginx; then
  echo "==> [nginx] 部署站点配置与证书"
  mkdir -p "$CERT_DIR"
  cp -a "$PKG/certs/live.rextian.com" "$CERT_DIR/"
  cp "$PKG/nginx-live.conf" "$NGX_CONF"
  rm -f /etc/nginx/sites-enabled/default
  nginx -t
  systemctl enable nginx
fi

# ---- 6. 启动 ----
if has start; then
  echo "==> [start] 启动服务"
  systemctl restart zlm; sleep 3
  systemctl restart livems; sleep 3
  systemctl restart nginx
fi

# ---- 7. 健康检查 ----
if has check; then
  echo "==> [check] 健康检查"
  for s in mysql redis-server zlm livems nginx; do
    printf '  %-14s %s\n' "$s" "$(systemctl is-active $s)"
  done
  echo "  livems /health: $(curl -fsS --max-time 5 http://127.0.0.1:8090/health || echo FAIL)"
  echo "  nginx  /health: $(curl -fsS --max-time 5 http://127.0.0.1:51880/health || echo FAIL)"
  echo "  前端 index:     $(curl -fsS --max-time 5 -o /dev/null -w '%{http_code}' http://127.0.0.1:51880/)"
  echo "  ZLM API:        $(curl -fsS --max-time 5 "http://127.0.0.1:8080/index/api/getServerConfig?secret=$(grep -E '^secret=' $ZLM_DIR/conf/config.ini | cut -d= -f2)" | head -c 80)..."
  echo "  监听端口:"; ss -ltnp | grep -E ':(1935|8080|8090|51880|51443|3306|6379|554) ' | awk '{print "    "$4, $6}'
fi
echo "==> 完成 ($STEPS)"

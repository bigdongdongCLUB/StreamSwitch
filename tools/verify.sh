#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 部署后端到端验证（在目标主机执行）
set -uo pipefail
ZLM_SECRET=$(grep -E '^secret=' $ZLM_DIR/conf/config.ini | cut -d= -f2 | tr -d '\r ')
APP=$APP_DIR
j() { python3 -c 'import sys,json; d=json.load(sys.stdin); print(json.dumps(d, ensure_ascii=False)[:'"${1:-400}"'])' 2>/dev/null || cat; }

echo "### 1. 服务状态"
for s in mysql redis-server $SVC_ZLM $SVC_APP nginx; do printf '  %-14s %s\n' "$s" "$(systemctl is-active $s)"; done
echo "  now: $(date -u +%FT%TZ)  epoch=$(date +%s)"

echo "### 2. ZLM API"
curl -s "http://127.0.0.1:8080/index/api/version?secret=$ZLM_SECRET" | j 300; echo
echo "  在线媒体流:"
curl -s "http://127.0.0.1:8080/index/api/getMediaList?secret=$ZLM_SECRET" \
  | python3 -c 'import sys,json
d=json.load(sys.stdin)
for m in d.get("data") or []:
    print("   ", m["schema"], m["app"]+"/"+m["stream"], "readers=%s" % m.get("readerCount"), "origin=%s" % m.get("originTypeStr"), [ (t.get("codec_id_name"), t.get("width"), t.get("height")) for t in m.get("tracks",[]) ])
print("    (共 %d 条)" % len(d.get("data") or []))'

echo "### 3. 制播任务自动恢复（ffmpeg 轮播回推）"
pgrep -af 'ffmpeg' | grep -v pgrep | cut -c1-220 || echo "  无 ffmpeg 进程"

echo "### 4. 数据库中的流状态"
$MYSQL -e \
  "SELECT id,name,app,stream,online,publish_ip,video_codec,width,height,reader_count FROM stream; SELECT id,name,type,status,pid,restart_count FROM studio_task\G" 2>/dev/null | grep -v '^\*\*\*'

echo "### 5. 后台 API（经 Nginx 51880）"
echo -n "  /health:            "; curl -s http://127.0.0.1:51880/health; echo
echo -n "  登录(错误密码):     "; curl -s -X POST http://127.0.0.1:51880/api/v1/auth/login -H 'Content-Type: application/json' -d '{"username":"admin","password":"wrong-password"}'; echo
echo -n "  未登录访问 /streams: "; curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:51880/api/v1/streams
echo -n "  前端 index.html:    "; curl -s http://127.0.0.1:51880/ | grep -oE '<title>[^<]*</title>'
echo -n "  HTTPS 51443 (-k):   "; curl -sk -o /dev/null -w '%{http_code} %{ssl_verify_result}\n' https://127.0.0.1:51443/health
echo -n "  证书主题:           "; echo | openssl s_client -connect 127.0.0.1:51443 -servername "${DDNS_DOMAIN:-localhost}" 2>/dev/null | openssl x509 -noout -subject -enddate 2>/dev/null | tr '\n' ' '; echo

echo "### 6. 播放鉴权（无令牌应被拒绝）"
echo -n "  HTTP-FLV 无 token:  "; curl -s -o /dev/null -w '%{http_code}\n' --max-time 5 "http://127.0.0.1:51880/live/live/vd01.live.flv"
echo -n "  HLS 无 token:       "; curl -s -o /dev/null -w '%{http_code}\n' --max-time 5 "http://127.0.0.1:51880/live/live/vd01/hls.m3u8"
echo -n "  旧 token(已过期):   "; curl -s -o /dev/null -w '%{http_code}\n' --max-time 5 "http://127.0.0.1:51880/live/live/vd01.live.flv?token=GBQQGTp7WLtQ9lJi_uWKS23LZ-DDra3M"

echo "### 7. 日志尾部"
echo "  -- livems:"; tail -n 15 $APP/backend/logs/livems.log | cut -c1-200
echo "  -- zlm:";    grep -vE '^\s*$' $ZLM_DIR/log/stdout.log | tail -n 6 | cut -c1-200
echo "  -- nginx error:"; tail -n 3 /var/log/nginx/streamswitch.error.log 2>/dev/null
echo "### 8. 资源占用"
uptime; free -h | head -2; ps -o pid,pcpu,pmem,rss,comm -C MediaServer,livems,ffmpeg

#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 流状态开关诊断：数据库 status / ZLM 实际在线 / 最近 hook 日志
M="$MYSQL"
S=$(grep -E '^secret=' $ZLM_DIR/conf/config.ini | cut -d= -f2 | tr -d '\r ')
echo "--- DB stream:"; $M -e "SELECT id,name,app,stream,status,online,publish_ip,FROM_UNIXTIME(publish_time) pt FROM stream" 2>/dev/null
echo "--- DB studio_task:"; $M -e "SELECT id,name,app,stream,status,run_status,pid,retry_count,retry_times,LEFT(last_error,60) err FROM studio_task" 2>/dev/null
echo "--- ZLM 在线流 (rtmp):"
curl -s "http://127.0.0.1:8080/index/api/getMediaList?secret=$S&schema=rtmp" | python3 -c '
import sys,json; d=json.load(sys.stdin)
for m in d.get("data") or []: print("   ", m["app"]+"/"+m["stream"], "origin=%s" % m.get("originTypeStr"), "url=%s" % m.get("originUrl",""))'
echo "--- ffmpeg:"; pgrep -af "bin/tools/ffmpeg" | grep -oE "rtmp://[^ ]+"
echo "--- 最近 hook/toggle 日志:"
grep -E "on_publish|toggle|禁用|推流被拒|room02|流上线|流下线|制播任务" $APP_DIR/backend/logs/livems.log | tail -n 20 | cut -c1-200

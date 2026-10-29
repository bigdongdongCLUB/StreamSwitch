#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 端到端播放测试：重置 admin 密码 -> 登录 -> 生成播放令牌 -> HTTP-FLV / HLS 实际拉流
set -uo pipefail
APP=$APP_DIR
BASE=http://127.0.0.1:$WEB_PORT
jq_() { python3 -c "import sys,json; d=json.load(sys.stdin); print($1)"; }

echo "### 1. 使用 .env.local 中的管理员账号登录（不再重置密码）"

echo "### 2. 登录"
LOGIN=$(curl -s -X POST $BASE/api/v1/auth/login -H 'Content-Type: application/json' -d "$LOGIN_JSON")
echo "  $(echo "$LOGIN" | cut -c1-160)..."
JWT=$(echo "$LOGIN" | jq_ 'd["data"]["token"]') || { echo "登录失败"; exit 1; }
AUTH="Authorization: Bearer $JWT"
echo "  JWT 长度: ${#JWT}"
echo -n "  /auth/profile: "; curl -s -H "$AUTH" $BASE/api/v1/auth/profile | jq_ 'd["data"]["username"], d["data"].get("nickname"), d["data"].get("roles")'

echo "### 3. 后台生成的播放地址（流 id=30 live/vd01）"
curl -s -H "$AUTH" "$BASE/api/v1/streams/30/urls" | python3 -c '
import sys,json; d=json.load(sys.stdin)["data"]
print(json.dumps(d, ensure_ascii=False, indent=1)[:1500])'

echo "### 4. 生成播放令牌（1 小时）"
TK=$(curl -s -X POST -H "$AUTH" -H 'Content-Type: application/json' $BASE/api/v1/tokens -d '{"stream_id":30,"expire_sec":3600}')
echo "  $(echo "$TK" | cut -c1-300)"
TOKEN=$(echo "$TK" | jq_ 'd["data"]["token"]') || { echo "生成令牌失败"; exit 1; }
echo "  token=$TOKEN"
echo -n "  Redis 中的令牌: "; redis-cli --scan --pattern "livems:*$TOKEN*" | head -1; redis-cli TTL "$(redis-cli --scan --pattern "livems:*$TOKEN*" | head -1)"

echo "### 5. HTTP-FLV 实际拉流（8 秒）"
FLV="$BASE/live/vd01.live.flv?token=$TOKEN"
curl -s --max-time 8 -o /tmp/test.flv -w '  HTTP %{http_code}, 收到 %{size_download} 字节, 速率 %{speed_download} B/s\n' "$FLV"
echo -n "  文件头: "; head -c 3 /tmp/test.flv; echo "  ($(stat -c %s /tmp/test.flv) 字节)"
echo "  ffprobe 解析:"; ffprobe -v error -show_entries stream=codec_type,codec_name,width,height,r_frame_rate,sample_rate -of compact /tmp/test.flv | sed 's/^/    /'
echo "  ffprobe 直连拉流 3s:"; timeout 20 ffprobe -v error -rw_timeout 10000000 -show_entries format=format_name,bit_rate -of compact "$FLV" 2>&1 | sed 's/^/    /'

echo "### 6. HLS 实际拉流"
M3U8="$BASE/live/vd01/hls.m3u8?token=$TOKEN"
rm -f /tmp/hls.cookie
curl -s --max-time 8 -c /tmp/hls.cookie -w '  m3u8 HTTP %{http_code}\n' -o /tmp/hls.m3u8 "$M3U8"
sed 's/^/    /' /tmp/hls.m3u8 | head -12
SEG=$(grep -vE '^#|^\s*$' /tmp/hls.m3u8 | head -1)
if [[ -n "$SEG" ]]; then
  case "$SEG" in http*) SEGURL="$SEG";; /*) SEGURL="$BASE$SEG";; *) SEGURL="$BASE/live/vd01/$SEG";; esac
  curl -s --max-time 8 -b /tmp/hls.cookie -o /tmp/seg.ts -w "  切片 $SEG: HTTP %{http_code}, %{size_download} 字节\n" "$SEGURL"
  echo -n "  切片 ffprobe: "; ffprobe -v error -show_entries stream=codec_type,codec_name,width,height -of compact /tmp/seg.ts | tr '\n' ' '; echo
fi
echo "  ffplay 式整体校验（ffprobe 读 m3u8 流）:"
timeout 25 ffprobe -v error -show_entries format=format_name:stream=codec_type,codec_name -of compact "$M3U8" 2>&1 | sed 's/^/    /'

echo "### 7. 并发拉流 10 秒并观察后台统计"
curl -s --max-time 10 -o /dev/null "$FLV" &
curl -s --max-time 10 -o /dev/null "$FLV" &
sleep 4
S=$(grep -E '^secret=' $ZLM_DIR/conf/config.ini | cut -d= -f2 | tr -d '\r ')
echo -n "  ZLM rtmp 流 readerCount: "; curl -s "http://127.0.0.1:8080/index/api/getMediaList?secret=$S&schema=rtmp&app=live&stream=vd01" | jq_ '[m["readerCount"] for m in d["data"]]'
echo -n "  后台 /streams/30 info: "; curl -s -H "$AUTH" "$BASE/api/v1/streams/30/info" | cut -c1-300; echo
wait
echo -n "  数据库 stream 行: "; $MYSQL -N -e "SELECT online,reader_count,total_reader,total_bytes FROM stream WHERE id=30" 2>/dev/null

echo "### 8. livems 日志中的 on_play 放行记录"
grep -E 'on_play' $APP/backend/logs/livems.log | tail -n 6 | cut -c1-200
rm -f /tmp/test.flv /tmp/seg.ts /tmp/hls.m3u8 /tmp/hls.cookie
echo "### E2E_DONE"

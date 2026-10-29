#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 观看人数诊断：数据库 reader_count vs ZLM 实际
M="$MYSQL"
S=$(grep -E '^secret=' $ZLM_DIR/conf/config.ini | cut -d= -f2 | tr -d '\r ')
echo "--- DB stream（online / reader_count / bitrate）:"
$M -e "SELECT id,name,app,stream,online,reader_count,total_reader,bitrate,FROM_UNIXTIME(publish_time) pt,FROM_UNIXTIME(offline_time) ot,FROM_UNIXTIME(updated_at) ut FROM stream" 2>/dev/null
echo "--- ZLM 当前媒体列表:"
curl -s "http://127.0.0.1:8080/index/api/getMediaList?secret=$S" | python3 -c '
import sys,json; d=json.load(sys.stdin); rows=d.get("data") or []
for m in rows: print("   ", m["schema"], m["app"]+"/"+m["stream"], "readers=%s" % m.get("readerCount"), "totalReaders=%s" % m.get("totalReaderCount"))
print("    (共 %d 条)" % len(rows))'
echo "--- ZLM 全部会话（可能残留的播放连接）:"
curl -s "http://127.0.0.1:8080/index/api/getAllSession?secret=$S" | python3 -c '
import sys,json; d=json.load(sys.stdin); rows=d.get("data") or []
for s in rows: print("   ", s.get("typeid"), s.get("peer_ip"), s.get("peer_port"), "->", s.get("local_port"))
print("    (共 %d 条)" % len(rows))'
echo "--- 最近 reader/on_play/none_reader 日志:"
grep -E "on_play|none_reader|流下线|流上线|同步" $APP_DIR/backend/logs/livems.log | tail -n 12 | cut -c1-180

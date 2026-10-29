#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 清理迁移带来的孤儿播放令牌：指向已不存在的流 live/src45(25) / live/rtmps_test(28)
# 步骤：备份 -> 校验 stream 表确实没有这两条流 -> 删 Redis key -> 删 MySQL 记录 -> 校验
set -euo pipefail
M="$MYSQL"
WHERE="stream_id IN (25,28) AND app='live' AND stream IN ('src45','rtmps_test')"
BK=/root/play_token-orphans-$(date +%Y%m%d-%H%M%S).sql

echo "### 0. 删除前状态"
$M -e "SELECT app, stream, stream_id, COUNT(*) n, FROM_UNIXTIME(MIN(created_at)) first_created, FROM_UNIXTIME(MAX(expire_at)) last_expire FROM play_token GROUP BY app, stream, stream_id" 2>/dev/null

echo "### 1. 确认这两条流已不在流表（必须为 0）"
N=$($M -N -e "SELECT COUNT(*) FROM stream WHERE id IN (25,28) OR (app='live' AND stream IN ('src45','rtmps_test'))" 2>/dev/null)
[[ "$N" == "0" ]] || { echo "流表中仍存在 src45/rtmps_test，中止"; exit 1; }
echo "  OK，流表中不存在"

echo "### 2. 备份待删记录 -> $BK"
mysqldump -h127.0.0.1 -u$DB_USER -p$DB_PASS --no-create-info --where="$WHERE" $DB_NAME play_token > "$BK" 2>/dev/null
echo "  备份 $(grep -c 'INSERT' "$BK") 条 INSERT 语句，$(wc -c < "$BK") 字节"

echo "### 3. 清 Redis"
TOKENS=$($M -N -e "SELECT token FROM play_token WHERE $WHERE" 2>/dev/null)
CNT=0; HIT=0
for t in $TOKENS; do
  CNT=$((CNT+1))
  for k in $(redis-cli --scan --pattern "*playtoken:$t"); do
    redis-cli DEL "$k" >/dev/null && HIT=$((HIT+1))
  done
done
echo "  令牌 $CNT 个，Redis 中命中并删除 $HIT 个 key（其余在 Redis 中本就不存在/已过期）"

echo "### 4. 删 MySQL"
$M -e "DELETE FROM play_token WHERE $WHERE" 2>/dev/null
echo "  已执行"

echo "### 5. 删除后状态"
$M -e "SELECT app, stream, stream_id, COUNT(*) n FROM play_token GROUP BY app, stream, stream_id" 2>/dev/null
echo "  Redis 剩余 playtoken key: $(redis-cli --scan --pattern '*playtoken:*' | wc -l)"
echo "  MySQL 剩余令牌: $($M -N -e 'SELECT COUNT(*) FROM play_token' 2>/dev/null)"
echo "### DONE  (回滚: mysql ... streamswitch < $BK)"

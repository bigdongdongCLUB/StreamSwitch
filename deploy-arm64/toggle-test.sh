#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 流状态开关端到端测试（Mac 上运行）：
#   用一路 ffmpeg 测试推流 → 禁用该流 → 应被立即踢掉且重连被拒 → 启用 → 可再次推流
set -uo pipefail
IP=$HOST_IP; B=$BASE_URL
K=$SSH_KEY; H=$SSH_HOST
j() { python3 -c "import sys,json; d=json.load(sys.stdin); print($1)"; }
JWT=$(curl -s -X POST $B/api/v1/auth/login -H 'Content-Type: application/json' \
      -d "$LOGIN_JSON" | j 'd["data"]["token"]') || exit 1
AUTH="Authorization: Bearer $JWT"
FF=$(command -v ffmpeg || echo "")

echo "### 1. 创建临时测试流 live/toggletest"
# 若上次测试遗留了同名流则直接复用
SID=$(curl -s -H "$AUTH" "$B/api/v1/streams?keyword=toggletest&page_size=5" | python3 -c 'import sys,json; d=json.load(sys.stdin)["data"]; r=d if isinstance(d,list) else d.get("list",[]); print(next((x["id"] for x in r if x["stream"]=="toggletest"), ""))')
if [[ -z "$SID" ]]; then
  R=$(curl -s -X POST -H "$AUTH" -H 'Content-Type: application/json' $B/api/v1/streams -d '{"name":"开关测试(临时)","app":"live","stream":"toggletest","remark":"toggle-test"}')
  SID=$(echo "$R" | j 'd["data"]["stream"]["id"]') || { echo "  创建失败: $R"; exit 1; }
fi
KEY=$(curl -s -H "$AUTH" $B/api/v1/streams/$SID/info | j 'd["data"]["push_key"]')
echo "  id=$SID push_key=$KEY"
PUSH="rtmp://$IP:1935/live/toggletest?key=$KEY"

push() {  # 在主机上用系统 ffmpeg 推一路测试图（本机不一定有 ffmpeg）
  ssh -i $K $H "nohup ffmpeg -hide_banner -loglevel error -re -f lavfi -i testsrc=size=640x360:rate=25 -f lavfi -i sine=frequency=440 -c:v libx264 -preset ultrafast -tune zerolatency -g 50 -c:a aac -f flv '$PUSH' > /root/toggletest-ffmpeg.log 2>&1 & echo \$!"
}
online() { curl -s -H "$AUTH" $B/api/v1/streams/$SID/info | j 'str(d["data"]["online"]) + " status=" + str(d["data"]["status"])'; }
ffalive() { ssh -i $K $H "pgrep -f 'toggletest' >/dev/null && echo 运行中 || echo 已退出; tail -n 2 /root/toggletest-ffmpeg.log 2>/dev/null | cut -c1-160"; }

echo "### 2. 开始推流"
PID=$(push); sleep 5
echo "  推流进程 pid=${PID}; 流状态 online=$(online)"

echo "### 3. 禁用该流（开关关闭）"
curl -s -X POST -H "$AUTH" $B/api/v1/streams/$SID/toggle; echo
sleep 2
echo "  流状态 online=$(online)"
echo "  推流端: $(ffalive)"

echo "### 4. 禁用状态下再次尝试推流（应被 on_publish 拒绝）"
PID2=$(push); sleep 5
echo "  推流端: $(ffalive)"
echo "  流状态 online=$(online)"

echo "### 5. 重新启用并推流（应恢复在线）"
curl -s -X POST -H "$AUTH" $B/api/v1/streams/$SID/toggle; echo
PID3=$(push); sleep 5
echo "  流状态 online=$(online)"

echo "### 6. 清理：停止推流、删除测试流"
ssh -i $K $H "pkill -f toggletest; rm -f /root/toggletest-ffmpeg.log" 2>/dev/null
sleep 1
curl -s -X DELETE -H "$AUTH" $B/api/v1/streams/$SID | cut -c1-80; echo
echo "### 相关后端日志"
ssh -i $K $H '. /root/livems-pkg/env.sh; grep -E "toggletest" $APP_DIR/backend/logs/livems.log | grep -vE "GET /api" | tail -n 12 | cut -c1-170'
echo "### TEST_DONE"

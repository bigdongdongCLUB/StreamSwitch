#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 定位上传慢的环节：裸 TCP  vs  直连后端 8090  vs  经 Nginx 51880（Mac 上运行）
set -uo pipefail
K=$SSH_KEY; H=$SSH_HOST; IP=$HOST_IP
SIZE=${SIZE:-300000000}
F="${TMPDIR:-/tmp}/bench.mp4"
[[ -f "$F" && $(stat -f %z "$F") -eq $SIZE ]] || head -c "$SIZE" /dev/zero > "$F"
j() { python3 -c "import sys,json; d=json.load(sys.stdin); print($1)"; }
MB=$((SIZE / 1000000))

echo "### 1. 裸 TCP ${MB}MB  Mac -> NanoPi (python socket)"
scp -q -i $K tools/tcp-srv.py $H:/root/livems-pkg/
ssh -i $K $H '. /root/livems-pkg/env.sh; nohup python3 /root/livems-pkg/tcp-srv.py 5202 > /root/tcp-srv.log 2>&1 &'
sleep 1
python3 - "$IP" "$SIZE" <<'EOF'
import socket, sys, time
ip, size = sys.argv[1], int(sys.argv[2])
buf = b"\0" * (1 << 20)
s = socket.create_connection((ip, 5202))
t = time.time(); sent = 0
while sent < size:
    s.sendall(buf); sent += len(buf)
s.close(); dt = time.time() - t
print("  send: %.0f MB in %.2fs = %.1f MB/s (%.0f Mbps)" % (sent/1e6, dt, sent/1e6/dt, sent*8/1e6/dt))
EOF
sleep 1; ssh -i $K $H '. /root/livems-pkg/env.sh; echo "  $(cat /root/tcp-srv.log)"'

echo "### 登录"
JWT=$(curl -s -X POST http://$IP:51880/api/v1/auth/login -H 'Content-Type: application/json' \
      -d "$LOGIN_JSON" | j 'd["data"]["token"]') || { echo "登录失败"; exit 1; }
AUTH="Authorization: Bearer $JWT"

echo "### 2. HTTP 上传 ${MB}MB 直连后端 :8090（绕过 Nginx）"
curl -s -o /dev/null -H "$AUTH" -F "file=@$F" -F type=video \
     -w '  HTTP %{http_code}  总耗时 %{time_total}s  上传速率 %{speed_upload} B/s\n' "http://$IP:8090/api/v1/studio/materials"

echo "### 3. HTTP 上传 ${MB}MB 经 Nginx :51880"
curl -s -o /dev/null -H "$AUTH" -F "file=@$F" -F type=video \
     -w '  HTTP %{http_code}  总耗时 %{time_total}s  上传速率 %{speed_upload} B/s\n' "http://$IP:51880/api/v1/studio/materials"

echo "### 4. HTTP 上传 ${MB}MB 经 Nginx，关闭 Expect:100-continue"
curl -s -o /dev/null -H "$AUTH" -H 'Expect:' -F "file=@$F" -F type=video \
     -w '  HTTP %{http_code}  总耗时 %{time_total}s  上传速率 %{speed_upload} B/s\n' "http://$IP:51880/api/v1/studio/materials"

echo "### 清理测试素材"
IDS=$(curl -s -H "$AUTH" "http://$IP:51880/api/v1/studio/materials?type=video" | python3 -c '
import sys,json; d=json.load(sys.stdin)["data"]
rows = d if isinstance(d, list) else d.get("list", [])
print(" ".join(str(r["id"]) for r in rows if r["name"] == "bench"))')
for id in $IDS; do curl -s -o /dev/null -X DELETE -H "$AUTH" "http://$IP:51880/api/v1/studio/materials/$id"; done
echo "  已删除 bench 素材: ${IDS:-无}"
echo "### 后端日志"
ssh -i $K $H '. /root/livems-pkg/env.sh; grep -E "POST /api/v1/studio/materials" $APP_DIR/backend/logs/livems.log | tail -n 3 | cut -c1-120'
echo "### BENCH_DONE"

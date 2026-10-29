#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 上传期间采集 livems 的 CPU profile 与 goroutine 栈（Mac 上运行；需后端开启 pprof_addr=127.0.0.1:6060）
set -uo pipefail
K=$SSH_KEY; H=$SSH_HOST; IP=$HOST_IP
F="${TMPDIR:-/tmp}/bench.mp4"; [[ -f "$F" ]] || head -c 300000000 /dev/zero > "$F"
OUT=deploy-arm64/prof; mkdir -p "$OUT"
j() { python3 -c "import sys,json; d=json.load(sys.stdin); print($1)"; }

JWT=$(curl -s -X POST http://$IP:51880/api/v1/auth/login -H 'Content-Type: application/json' \
      -d "$LOGIN_JSON" | j 'd["data"]["token"]') || { echo "登录失败"; exit 1; }
AUTH="Authorization: Bearer $JWT"

echo "### pprof 可达性"; ssh -i $K $H '. /root/livems-pkg/env.sh; curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:6060/debug/pprof/'

echo "### 后台发起 300MB 直连上传（约 60s），3s 后开始采样 20s"
curl -s -o /dev/null -H "$AUTH" -F "file=@$F" -F type=video \
     -w '  上传完成: HTTP %{http_code} %{time_total}s %{speed_upload} B/s\n' "http://$IP:8090/api/v1/studio/materials" &
CPID=$!
sleep 3
ssh -i $K $H '. /root/livems-pkg/env.sh; curl -s -o /root/cpu.pprof "http://127.0.0.1:6060/debug/pprof/profile?seconds=20" && curl -s "http://127.0.0.1:6060/debug/pprof/goroutine?debug=2" > /root/goroutines.txt && ls -la /root/cpu.pprof && echo "  采样完成"'
wait $CPID
scp -q -i $K $H:/root/cpu.pprof $H:/root/goroutines.txt "$OUT/"

echo "### CPU profile TOP（按 flat）"
GOCACHE="${TMPDIR:-/tmp}/gocache" go tool pprof -top -nodecount=28 "$OUT/cpu.pprof" 2>/dev/null | head -40
echo "### CPU profile TOP（按 cum，看调用链）"
GOCACHE="${TMPDIR:-/tmp}/gocache" go tool pprof -top -cum -nodecount=28 "$OUT/cpu.pprof" 2>/dev/null | sed -n '7,40p'
echo "### 上传相关 goroutine 栈"
grep -n -B2 -A14 -E 'copyWithLimit|SaveMaterial|multipart|UploadMaterial' "$OUT/goroutines.txt" | head -90

echo "### 清理 bench 素材"
IDS=$(curl -s -H "$AUTH" "http://$IP:51880/api/v1/studio/materials?type=video" | python3 -c '
import sys,json; d=json.load(sys.stdin)["data"]
rows = d if isinstance(d, list) else d.get("list", [])
print(" ".join(str(r["id"]) for r in rows if r["name"] == "bench"))')
for id in $IDS; do curl -s -o /dev/null -X DELETE -H "$AUTH" "http://$IP:51880/api/v1/studio/materials/$id"; done
echo "  已删除: ${IDS:-无}"
echo "### PROFILE_DONE"

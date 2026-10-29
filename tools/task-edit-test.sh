#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 复现「编辑制播任务」保存：取用户现有任务，原样 PUT 回去（与前端点「确定」等价，不改任何配置）
set -uo pipefail
IP=$HOST_IP; B=$BASE_URL
JWT=$(curl -s -X POST $B/api/v1/auth/login -H 'Content-Type: application/json' \
      -d "$LOGIN_JSON" | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["token"])') || exit 1
AUTH="Authorization: Bearer $JWT"

echo "### 当前任务列表"
curl -s -H "$AUTH" "$B/api/v1/studio/tasks" > /tmp/tasks.json
python3 - <<'EOF'
import json
d = json.load(open('/tmp/tasks.json'))["data"]
rows = d if isinstance(d, list) else d.get("list", [])
for t in rows:
    print("  id=%s name=%s mode=%s out=%s/%s material=%s quality=%s crf=%s status=%s run=%s" %
          (t["id"], t["name"], t["mode"], t["app"], t["stream"], t["material_id"], t["quality_mode"], t["crf"], t["status"], t["run_status"]))
EOF

echo "### 原样保存第一个任务（PUT）"
python3 - "$B" "$JWT" <<'EOF'
import json, sys, urllib.request
B, JWT = sys.argv[1], sys.argv[2]
d = json.load(open('/tmp/tasks.json'))["data"]
rows = d if isinstance(d, list) else d.get("list", [])
if not rows:
    print("  无任务可测"); sys.exit(0)
t = rows[0]
keys = ["name","mode","material_id","src_url","app","stream","caption_enabled","caption_text","caption_size",
        "caption_color","caption_position","caption_speed","logo_material_id","logo_position","logo_opacity",
        "logo_width_pct","vbitrate","quality_mode","crf","status","retry_count"]
body = {k: t.get(k) for k in keys}
body["caption_enabled"] = bool(body["caption_enabled"])
req = urllib.request.Request(f"{B}/api/v1/studio/tasks/{t['id']}", data=json.dumps(body).encode(), method="PUT",
                             headers={"Content-Type": "application/json", "Authorization": "Bearer " + JWT})
try:
    with urllib.request.urlopen(req) as r:
        print("  HTTP", r.status, r.read().decode()[:200])
except urllib.error.HTTPError as e:
    print("  HTTP", e.code, e.read().decode()[:300])
EOF

echo "### 保存后进程状态（应为新 pid、running）"
sleep 2
curl -s -H "$AUTH" "$B/api/v1/studio/tasks" | python3 -c '
import sys,json; d=json.load(sys.stdin)["data"]; r=d if isinstance(d,list) else d.get("list",[])
for t in r: print("  id=%s run=%s pid=%s err=%s" % (t["id"], t["run_status"], t["pid"], (t.get("last_error") or "")[:80]))'
ssh -i $SSH_KEY $SSH_HOST '. /root/livems-pkg/env.sh; pgrep -af "bin/tools/ffmpeg" | cut -c1-90; grep -E "制播任务" $APP_DIR/backend/logs/livems.log | tail -n 4 | cut -c1-160'
echo "### 后端日志（转译记录）"
ssh -i $SSH_KEY $SSH_HOST '. /root/livems-pkg/env.sh; grep -E "转为中文提示|PUT /api/v1/studio/tasks" $APP_DIR/backend/logs/livems.log | tail -n 4 | cut -c1-220'
echo "### TEST_DONE"

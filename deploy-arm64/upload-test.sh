#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 在 Mac 上通过 Nginx(51880) 实测大文件素材上传（走与浏览器完全相同的链路）
# 用法: bash deploy-arm64/upload-test.sh <本地视频文件> [keep]   默认测完删除测试素材
set -uo pipefail
HOST=${HOST:-$BASE_URL}
FILE="${1:?用法: upload-test.sh <视频文件> [keep]}"
KEEP="${2:-}"
j() { python3 -c "import sys,json; d=json.load(sys.stdin); print($1)"; }

echo "### 登录"
JWT=$(curl -s -X POST $HOST/api/v1/auth/login -H 'Content-Type: application/json' \
      -d "$LOGIN_JSON" | j 'd["data"]["token"]') || { echo "登录失败"; exit 1; }
AUTH="Authorization: Bearer $JWT"
BEFORE=$(curl -s -H "$AUTH" "$HOST/api/v1/studio/materials?type=video" | j 'len(d["data"]) if isinstance(d["data"],list) else d["data"].get("total", len(d["data"].get("list",[])))')
echo "  上传前视频素材数: $BEFORE"

echo "### 上传 $(du -h "$FILE" | cut -f1) $(basename "$FILE")"
START=$(date +%s)
RESP=$(curl -s --max-time 900 -H "$AUTH" -F "file=@$FILE" -F "type=video" \
       -w '\n__HTTP:%{http_code} 上传耗时:%{time_total}s 速率:%{speed_upload}B/s' \
       "$HOST/api/v1/studio/materials")
END=$(date +%s)
echo "$RESP" | tail -1
BODY=$(echo "$RESP" | sed '$d')
echo "  响应: $(echo "$BODY" | cut -c1-300)"
NEWID=$(echo "$BODY" | j 'd["data"]["id"]' 2>/dev/null)
echo "  总耗时 $((END-START))s，新素材 id=${NEWID:-无}"

echo "### 服务端日志"
ssh -i $SSH_KEY $SSH_HOST \
  'grep -E "studio/materials|素材已上传" $APP_DIR/backend/logs/livems.log | tail -n 3 | cut -c1-200; echo "  nginx error 新增:"; tail -n 2 /var/log/nginx/streamswitch.error.log | cut -c1-160; echo "  内存:"; free -m | sed -n 2p; ls -la $APP_DIR/backend/data/studio/videos/ | tail -n +4'

if [[ -n "$NEWID" && "$KEEP" != "keep" ]]; then
  echo "### 清理测试素材 id=$NEWID"
  curl -s -X DELETE -H "$AUTH" "$HOST/api/v1/studio/materials/$NEWID" | cut -c1-120; echo
fi
echo "### TEST_DONE"

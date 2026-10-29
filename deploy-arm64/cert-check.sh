#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
set -uo pipefail
B=http://127.0.0.1:51880
J=$(curl -s -X POST $B/api/v1/auth/login -H 'Content-Type: application/json' -d "$LOGIN_JSON" | python3 -c 'import sys,json;print(json.load(sys.stdin)["data"]["token"])')
echo "--- GET /api/v1/certs"; curl -s -H "Authorization: Bearer $J" $B/api/v1/certs; echo
echo "--- DELETE /api/v1/certs/default"; curl -s -X DELETE -H "Authorization: Bearer $J" $B/api/v1/certs/default; echo
echo "--- 证书目录"; ls -la /www/server/panel/vhost/cert/ /www/server/panel/vhost/cert/*/
echo "--- nginx conf 中的证书引用"; grep -n ssl_certificate /etc/nginx/conf.d/streamswitch.conf
echo "--- livems 读取的 vhost 路径"; grep -n vhost_conf $APP_DIR/backend/configs/config.yaml

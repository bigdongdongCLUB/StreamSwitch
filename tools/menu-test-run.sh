#!/usr/bin/env bash
# 在测试主机上运行交互式菜单测试（expect），并用生成的新密码实际登录验证。Mac 上运行。
. "$(dirname "$0")/env.sh"
TMP=$(mktemp "${TMPDIR:-/tmp}/menurun.XXXXXX")
cat > "$TMP" <<'REMOTE'
command -v expect >/dev/null || DEBIAN_FRONTEND=noninteractive apt-get install -y -q expect >/dev/null 2>&1
rm -f /root/menu-result.txt
expect /root/menu-test.exp >/dev/null 2>&1; rc=$?
out=$(cat /root/menu-result.txt 2>/dev/null)
echo "$out"
echo "expect rc=$rc"
clean() { sed 's/\x1b\[[0-9;]*m//g' /root/menu-test.log | tr -d '\r'; }
echo "--- 菜单交互记录（节选）"
clean | grep -aE "请选择|ID +用户名|^ +[0-9]+ +[a-z]|要重置密码|已重置|新密码：|当前端口|端口未变化" | head -20
NP=$(echo "$out" | grep -oE 'newpw=\S+' | cut -d= -f2)
P=$(grep -E '^HTTP_PORT=' /opt/streamswitch/install.env | cut -d= -f2)
echo "--- 用菜单生成的新密码登录"
curl -s -X POST "http://127.0.0.1:$P/api/v1/auth/login" -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$NP\"}" | grep -o '"token":"[^"]\{12\}' && echo "  登录成功"
REMOTE
scp -q -i "$SSH_KEY" "$(dirname "$0")/menu-test.exp" "$SSH_HOST":/root/
scp -q -i "$SSH_KEY" "$TMP" "$SSH_HOST":/root/menu-test-remote.sh && rm -f "$TMP"
ssh -i "$SSH_KEY" "$SSH_HOST" 'bash /root/menu-test-remote.sh </dev/null'

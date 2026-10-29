#!/usr/bin/env bash
# 提交并推送前做敏感信息扫描（任何命中即中止）。用法: bash tools/git-push-checked.sh "提交说明"
#
# 扫描词表：
#   - 通用：PEM 私钥头（独占一行，排除前端 placeholder 文案）
#   - 站点私有：同目录 .leak-patterns.local（不入库，每行一个 ERE 正则：真实 IP / 密码 / 密钥 / MAC / 域名）
set -euo pipefail
cd "$(dirname "$0")/.."
MSG="${1:?用法: git-push-checked.sh \"提交说明\"}"
PATTERN='-----BEGIN (RSA |EC )?PRIVATE KEY-----$'
LOCAL=tools/.leak-patterns.local
if [[ -f "$LOCAL" ]]; then
  EXTRA=$(grep -vE '^\s*(#|$)' "$LOCAL" | paste -sd'|' -)
  [[ -n "$EXTRA" ]] && PATTERN="$PATTERN|$EXTRA"
else
  echo "提示: 未找到 ${LOCAL}，仅做通用扫描（建议把站点真实 IP/密码/密钥写入该文件）"
fi
git add -A
HITS=$(git diff --cached --name-only --diff-filter=ACMR | xargs grep -lIE "$PATTERN" 2>/dev/null || true)
if [[ -n "$HITS" ]]; then
  echo "!! 暂存内容命中敏感词，已中止："; echo "$HITS" | sed 's/^/   /'
  echo "$HITS" | xargs grep -nIE "$PATTERN" | cut -c1-140 | sed 's/^/     /'; exit 1
fi
ALL=$(git ls-files | xargs grep -lIE "$PATTERN" 2>/dev/null || true)
if [[ -n "$ALL" ]]; then
  echo "!! 已跟踪文件命中敏感词，已中止："; echo "$ALL" | sed 's/^/   /'; exit 1
fi
echo "敏感词扫描通过"
git commit -q -m "$MSG" -m "Co-Authored-By: Claude Opus 5.5 <noreply@anthropic.com>"
git push -q origin main
git log --oneline -2
REPO=$(gh repo view --json nameWithOwner -q .nameWithOwner)
echo "--- 远端文件数: $(gh api "repos/$REPO/git/trees/main?recursive=1" -q '[.tree[] | select(.type=="blob")] | length')"
echo "--- 远端敏感路径命中: $(gh api "repos/$REPO/git/trees/main?recursive=1" -q '.tree[].path' | grep -ciE 'certs/|livems_key|\.env\.local|\.local\.md|\.leak-patterns' || true)"
echo PUSH_DONE

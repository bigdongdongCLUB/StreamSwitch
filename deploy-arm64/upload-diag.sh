#!/usr/bin/env bash
. "$(dirname "$0")/env.sh"
# 素材上传问题诊断
APP=$APP_DIR
echo "--- livems log (materials/timeout/error):"
grep -nE "materials|素材|ffprobe|timeout|i/o|EOF|broken pipe| 5[0-9][0-9] " $APP/backend/logs/livems.log | tail -n 25 | cut -c1-260
echo "--- nginx error log:"
tail -n 15 /var/log/nginx/live.rextian.com.error.log | cut -c1-300
echo "--- nginx access: POST materials:"
grep -E "POST /api/v1/studio/materials" /var/log/nginx/live.rextian.com.log | tail -n 8 | cut -c1-220
echo "--- disk:"
df -h /www /tmp /var/lib/nginx 2>/dev/null
ls -la $APP/backend/data/studio/videos/ 2>&1
ls -la /tmp | grep -iE "multipart" | head
ls -la /var/lib/nginx/body/ 2>/dev/null | head
echo "--- disk write speed (200MB, fdatasync):"
dd if=/dev/zero of=/www/ddtest bs=1M count=200 conv=fdatasync 2>&1 | tail -1; rm -f /www/ddtest
echo "--- disk read speed (video file, direct):"
F=$(ls $APP/backend/data/studio/videos/*.mp4 2>/dev/null | head -1)
[ -n "$F" ] && dd if="$F" of=/dev/null bs=1M iflag=direct 2>&1 | tail -1
echo "--- ffprobe time on existing video:"
[ -n "$F" ] && { time ffprobe -v quiet -print_format json -show_format -show_streams "$F" >/dev/null; } 2>&1 | grep real
echo "--- load/mem:"
uptime; free -m | head -2
echo "--- ffmpeg studio process:"
pgrep -af "bin/tools/ffmpeg" | cut -c1-120 || true

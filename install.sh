#!/usr/bin/env bash
# =============================================================================
#  StreamSwitch 流媒体管理系统 —— 一键部署脚本
#
#  在线安装：  bash <(curl -fsSL https://raw.githubusercontent.com/bigdongdongCLUB/StreamSwitch/main/install.sh)
#  离线安装：  解压发布包后执行  sudo bash install.sh
#  安装后管理：sudo streamswitch
#
#  菜单：1 安装/升级   2 端口设置   3 卸载   4 重置管理员密码
#  也可直接带参数：install | port | uninstall | reset-password | status
#
#  无人值守（CI/批量部署）环境变量：
#    SS_YES=1                 所有提问采用默认值（卸载时视为已确认）
#    SS_HTTP_PORT / SS_HTTPS_PORT / SS_RTMP_PORT / SS_RTSP_PORT(0=关闭)
#    SS_MYSQL_ROOT_PASSWORD   本机 MySQL root 密码（root 无法免密登录时）
#    SS_REDIS_PASSWORD        本机 Redis 密码（设置了 requirepass 时）
#    SS_APT_MIRROR=tuna       apt 切换为清华镜像
#    SS_PACKAGE=<路径或URL>   指定安装包；SS_VERSION=vX.Y.Z 指定版本；GH_PROXY=<GitHub 加速前缀>
#    SS_FORCE=1               版本相同也重新安装（升级流程）
#    SS_PURGE=1               卸载时同时删除数据库与素材数据
#    SS_RESET_USER / SS_RESET_PASSWORD   重置密码的目标用户与新密码
# =============================================================================
set -Eeo pipefail

REPO="bigdongdongCLUB/StreamSwitch"
INSTALL_DIR="${SS_INSTALL_DIR:-/opt/streamswitch}"
ENV_FILE="$INSTALL_DIR/install.env"
SVC_APP="streamswitch"
SVC_ZLM="streamswitch-zlm"
NGX_CONF="/etc/nginx/conf.d/streamswitch.conf"
BIN_LINK="/usr/local/bin/streamswitch"
LOGROTATE="/etc/logrotate.d/streamswitch"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")" 2>/dev/null && pwd || pwd)"
# 统一使用 C.UTF-8：中文按字符处理，且 apt 输出为英文便于解析进度
case $(locale -a 2>/dev/null || true) in *C.utf8*|*C.UTF-8*) export LC_ALL=C.UTF-8 ;; esac

# ----------------------------------------------------------------------------- 输出
if [[ -t 1 ]]; then
  C_R=$'\e[31m'; C_G=$'\e[32m'; C_Y=$'\e[33m'; C_B=$'\e[36m'; C_W=$'\e[1m'; C_0=$'\e[0m'
else
  C_R=; C_G=; C_Y=; C_B=; C_W=; C_0=
fi
info() { echo "${C_B}==>${C_0} $*"; }
ok()   { echo "${C_G} ✔${C_0} $*"; }
warn() { echo "${C_Y} !${C_0} $*" >&2; }
die()  { echo "${C_R} ✘ $*${C_0}" >&2; exit 1; }
# 出错提示只在主进程打印一次（$(...) 子进程中的失败会冒泡到主进程）
trap 'rc=$?; if [[ $rc -ne 0 && $BASH_SUBSHELL -eq 0 && -z ${_SS_ERR:-} ]]; then _SS_ERR=1; echo "${C_R} ✘ 执行中断（第 $LINENO 行，退出码 $rc）。可重新运行本脚本，已完成的步骤会自动跳过。${C_0}" >&2; fi' ERR

# ----------------------------------------------------------------------------- 显示宽度 / 进度
SS_LOG=/tmp/streamswitch-install.log

# 终端显示宽度：中文等全角字符占 2 列。与 locale 无关，按 UTF-8 字节推算——
# 字符数 = 去掉续字节(0x80-0xBF)后的字节数；ASCII 1 字节宽 1 列，中文 3 字节宽 2 列。
# （只用于 ASCII + 中文的文本；制表符号 ═║ 是 3 字节但只占 1 列，不要传进来）
dwidth() {
  local bytes chars
  bytes=$(printf '%s' "$1" | LC_ALL=C wc -c)
  chars=$(printf '%s' "$1" | LC_ALL=C tr -d '\200-\277' | LC_ALL=C wc -c)
  echo $(( chars + (bytes - chars) / 2 ))
}
# repeat 字符 次数
repeat() { local s; printf -v s '%*s' "$2" ''; printf '%s' "${s// /$1}"; }

term_cols() {
  local c=
  [[ -r $TTY ]] && c=$( { stty size <$TTY; } 2>/dev/null | awk '{print $2}' || true)
  [[ $c =~ ^[0-9]+$ && $c -ge 40 ]] || c=80
  echo "$c"
}
fmt_secs() { printf '%02d:%02d' $(( $1 / 60 )) $(( $1 % 60 )); }
fmt_mb() { printf '%d.%dMB' $(( $1 / 1048576 )) $(( $1 % 1048576 * 10 / 1048576 )); }
apt_cache_bytes() { local x; x=$(du -sb /var/cache/apt/archives 2>/dev/null || true); echo "${x%%[[:space:]]*}"; }

# run_progress "说明" 命令 [参数...]
#   后台执行命令（输出写入 $SS_LOG），前台显示进度，避免长时间无输出被误认为卡死：
#   - 命令向 fd 3 写 apt 状态（-o APT::Status-Fd=3）时显示真实百分比与当前软件包；
#   - 否则显示旋转指示、已读取的软件源索引数或日志最新一行；
#   - 终端中原地刷新一行；非终端（日志/CI）每 15 秒输出一行。
run_progress() {
  local title=$1; shift
  local lg st pid t0=$SECONDS rc=0 i=0 tty= cols spin='|/-\' last=-15
  lg=$(mktemp /tmp/streamswitch-step.XXXXXX); st=$(mktemp /tmp/streamswitch-status.XXXXXX)
  [[ -t 1 ]] && tty=1
  cols=$(term_cols)
  "$@" >"$lg" 2>&1 3>"$st" &
  pid=$!
  # Ctrl+C：连同后台命令一起结束（非交互 shell 中后台进程默认忽略 SIGINT）
  trap 'kill "$pid" 2>/dev/null; wait "$pid" 2>/dev/null; cat "$lg" >>"$SS_LOG"; rm -f "$lg" "$st"; printf "\n"; die "已中断：$title"' INT TERM
  local line kind a p rest pct msg verb pkg n ll el bar fill head base cur
  base=$(apt_cache_bytes); base=${base:-0}
  while kill -0 "$pid" 2>/dev/null; do
    pct=; msg=
    line=$(tail -n 1 "$st" 2>/dev/null || true)
    IFS=: read -r kind a p rest <<<"$line"
    case $kind in
      dlstatus)
        pct=${p%%.*}
        if [[ $rest =~ file\ ([0-9]+)\ of\ ([0-9]+) ]]; then msg="下载软件包 ${BASH_REMATCH[1]}/${BASH_REMATCH[2]}"; else msg="下载软件包"; fi
        # apt 只在每个文件开始时更新百分比：附上实时已下载量与平均速度，大文件下载时也能看到在动
        cur=$(apt_cache_bytes)
        if [[ ${cur:-0} -gt $base ]]; then
          el=$(( SECONDS - t0 > 0 ? SECONDS - t0 : 1 ))
          msg="$msg · 已下载 $(fmt_mb $(( cur - base ))) · $(fmt_mb $(( (cur - base) / el )))/s"
        fi ;;
      pmstatus)
        pct=${p%%.*}
        case $rest in
          Preparing*) verb="准备" ;; Unpacking*) verb="解包" ;; Installing*) verb="安装" ;;
          Configuring*|Preparing\ to\ configure*) verb="配置" ;; Installed*) verb="已安装" ;;
          Removing*|Removed*) verb="移除" ;; *) verb="处理" ;;
        esac
        pkg=$a; msg="$verb $pkg" ;;
    esac
    if [[ -z $msg ]]; then
      n=$(grep -cE '^(Get|Hit|Ign|获取|命中|忽略):' "$lg" 2>/dev/null || true)
      ll=$(tail -n 1 "$lg" 2>/dev/null || true); ll=${ll##*$'\r'}
      case $ll in
        *"Waiting for cache lock"*|*"Could not get lock"*) msg="等待系统其他软件更新进程结束（如 unattended-upgrades）" ;;
        *"Reading package lists"*)      msg="读取软件包列表" ;;
        *"Building dependency tree"*)   msg="分析依赖关系" ;;
        *"Reading state information"*)  msg="读取已安装软件状态" ;;
        *"Connecting to"*|*"Waiting for headers"*) msg="连接软件源" ;;
        *"Selecting previously"*|*"Preparing to unpack"*|*"Unpacking"*) msg="解包软件包" ;;
        *"Setting up"*)                 msg="配置软件包" ;;
        *"Processing triggers"*)        msg="处理系统触发器" ;;
        *)
          if [[ ${n:-0} -gt 0 ]]; then msg="已读取 $n 个软件源索引"; else msg="准备中"; fi ;;
      esac
      if [[ $msg == "读取软件包列表" && ${n:-0} -gt 0 ]]; then msg="已读取 $n 个软件源索引，正在整理"; fi
    fi
    el=$(( SECONDS - t0 ))
    if [[ -n $tty ]]; then
      if [[ $pct =~ ^[0-9]+$ ]]; then
        (( pct > 100 )) && pct=100
        fill=$(( pct * 24 / 100 ))
        bar="[$(repeat '#' "$fill")$(repeat '.' $(( 24 - fill )))] $(printf '%3d%%' "$pct")"
      else
        bar="[$(repeat ' ' $(( i % 21 )))<=>$(repeat ' ' $(( 21 - i % 21 )))]     "
      fi
      head="  ${spin:i%4:1} $bar  "
      # 截断说明文字，保证整行不超过终端宽度（不换行才能原地刷新）
      local room=$(( cols - $(dwidth "$head") - 10 ))
      while [[ -n $msg ]] && (( $(dwidth "$msg") > room )); do msg=${msg:0:${#msg}-2}; done
      printf '\r%s%s  %s\033[K' "$head" "$msg" "$(fmt_secs "$el")"
    elif (( el - last >= 15 )); then
      last=$el
      echo "    … ${pct:+$pct% }${msg:-进行中}（已用时 $(fmt_secs "$el")）"
    fi
    sleep 0.5; i=$(( i + 1 ))
  done
  wait "$pid" || rc=$?
  trap - INT TERM
  cat "$lg" >>"$SS_LOG"; rm -f "$lg" "$st"
  [[ -n $tty ]] && printf '\r\033[K'
  if [[ $rc -eq 0 ]]; then ok "$title（用时 $(fmt_secs $(( SECONDS - t0 )))）"; fi
  return "$rc"
}

# ----------------------------------------------------------------------------- 交互
TTY=/dev/tty
interactive() { [[ "${SS_YES:-}" != "1" ]] && [[ -r $TTY ]] && { : <$TTY; } 2>/dev/null; }

# ask "提示" 默认值 → 输出答案
ask() {
  local prompt=$1 def=${2:-} ans
  if interactive; then
    read -r -p "$prompt${def:+ [$def]}: " ans <$TTY || true
    echo "${ans:-$def}"
  else
    echo "$def"
  fi
}
# ask_yes "提示" y|n → 返回 0/1
ask_yes() {
  local prompt=$1 def=${2:-n} ans hint="[y/N]"
  [[ $def == y ]] && hint="[Y/n]"
  if interactive; then
    read -r -p "$prompt $hint: " ans <$TTY || true
    ans=${ans:-$def}
  else
    ans=$def
  fi
  [[ ${ans,,} == y || ${ans,,} == yes ]]
}
# ask_secret "提示" → 输出（不回显）
ask_secret() {
  local prompt=$1 ans
  if interactive; then
    read -r -s -p "$prompt: " ans <$TTY || true
    echo >&2
    echo "$ans"
  fi
}
pause() { interactive && read -r -p "按回车键返回菜单..." _ <$TTY || true; }

# ----------------------------------------------------------------------------- 环境检测
require_root() { [[ $EUID -eq 0 ]] || die "请使用 root 运行（例如：sudo bash $0）"; }

detect_arch() {
  case "$(uname -m)" in
    x86_64|amd64) ARCH=amd64 ;;
    aarch64|arm64) ARCH=arm64 ;;
    *) die "暂不支持的 CPU 架构：$(uname -m)（仅支持 x86_64 / aarch64）" ;;
  esac
}

detect_os() {
  [[ -r /etc/os-release ]] || die "无法识别操作系统（缺少 /etc/os-release）"
  # shellcheck disable=SC1091
  . /etc/os-release
  OS_ID=${ID:-unknown}; OS_VER=${VERSION_ID:-0}; OS_NAME=${PRETTY_NAME:-$OS_ID}
  command -v apt-get >/dev/null || die "当前系统 $OS_NAME 不是 apt 系发行版，暂不支持（支持 Ubuntu 22.04+ / Debian 12+）"
  local major=${OS_VER%%.*}
  case "$OS_ID" in
    ubuntu) [[ $major -ge 22 ]] || die "Ubuntu 版本过低（$OS_VER），需要 22.04 或更高" ;;
    debian) [[ $major -ge 12 ]] || die "Debian 版本过低（$OS_VER），需要 12 或更高" ;;
    *) warn "未经测试的发行版：$OS_NAME（按 Debian/Ubuntu 方式安装）"
       ask_yes "是否继续" n || exit 1 ;;
  esac
}

lan_ip() {
  local ip
  ip=$(ip -4 route get 1.1.1.1 2>/dev/null | awk '{for(i=1;i<=NF;i++) if($i=="src"){print $(i+1); exit}}' || true)
  [[ -z $ip ]] && ip=$(hostname -I 2>/dev/null | awk '{print $1}' || true)
  echo "${ip:-127.0.0.1}"
}

# 注意：不能写成 ss | grep -q —— pipefail 下 grep 提前退出会让 ss 收到 SIGPIPE，把“占用”误判为“空闲”
port_in_use() { [[ -n $(ss -ltnH "sport = :$1" 2>/dev/null || true) ]]; }

# 选端口：提示输入并校验格式与占用（own=当前已由本系统占用的端口，视为可用）
pick_port() {
  local label=$1 def=$2 envv=$3 own=${4:-} allow_zero=${5:-} p
  p=${!envv:-}
  while true; do
    [[ -z $p ]] && p=$(ask "$label" "$def")
    if [[ $allow_zero == 1 && $p == 0 ]]; then echo 0; return; fi
    if ! [[ $p =~ ^[0-9]+$ ]] || (( p < 1 || p > 65535 )); then
      warn "端口无效：$p"; p=; interactive || die "$label 无效"; continue
    fi
    if [[ $p != "$own" ]] && port_in_use "$p"; then
      warn "端口 $p 已被占用：$(ss -ltnpH "sport = :$p" 2>/dev/null | grep -oE 'users:\(\("[^"]+' | head -1 | cut -d'"' -f2)"
      p=; interactive || die "$label 端口冲突，请通过环境变量 $envv 指定其他端口"; continue
    fi
    echo "$p"; return
  done
}

# 等待端口释放（nginx reload 后旧监听由 master 异步关闭），最多约 5 秒
wait_port_free() { local i; for i in $(seq 1 10); do port_in_use "$1" || return 0; sleep 0.5; done; return 1; }

# 内部端口：从默认值起找一个空闲端口
free_port() {
  local p=$1 own=${2:-}
  while [[ $p != "$own" ]] && port_in_use "$p"; do p=$((p + 1)); done
  echo "$p"
}

rand_hex() { openssl rand -hex "${1:-16}" 2>/dev/null || head -c "${1:-16}" /dev/urandom | od -An -tx1 | tr -d ' \n'; }

installed() { [[ -f $ENV_FILE && -x $INSTALL_DIR/backend/bin/livems ]]; }
load_env() {
  installed || die "尚未安装 StreamSwitch，请先选择「1. 安装」"
  set -a; . "$ENV_FILE"; set +a
  # 兼容旧版本 install.env 缺少的键
  : "${HTTP_PORT:=51880}" "${HTTPS_PORT:=51443}" "${RTMP_PORT:=1935}" "${RTSP_PORT:=554}"
  : "${APP_PORT:=18090}" "${ZLM_HTTP_PORT:=18080}" "${DB_HOST:=127.0.0.1}" "${DB_PORT:=3306}"
  : "${REDIS_HOST:=127.0.0.1}" "${REDIS_PORT:=6379}" "${DB_SERVICE:=mysql}"
}
rtsp_txt() { [[ ${RTSP_PORT:-0} != 0 ]] && echo "、$RTSP_PORT" || true; }

save_env() {
  umask 077
  cat >"$ENV_FILE" <<EOF
# StreamSwitch 安装参数（由部署脚本维护，请勿手动修改；端口请用 streamswitch 菜单调整）
HTTP_PORT=$HTTP_PORT
HTTPS_PORT=$HTTPS_PORT
RTMP_PORT=$RTMP_PORT
RTSP_PORT=$RTSP_PORT
APP_PORT=$APP_PORT
ZLM_HTTP_PORT=$ZLM_HTTP_PORT
DB_HOST=$DB_HOST
DB_PORT=$DB_PORT
DB_NAME=$DB_NAME
DB_USER=$DB_USER
DB_PASS=$DB_PASS
REDIS_HOST=$REDIS_HOST
REDIS_PORT=$REDIS_PORT
REDIS_PASS=$REDIS_PASS
ZLM_SECRET=$ZLM_SECRET
JWT_SECRET=$JWT_SECRET
DB_SERVICE=$DB_SERVICE
EOF
  umask 022
}

# ----------------------------------------------------------------------------- 依赖安装
switch_apt_mirror() {
  local m=https://mirrors.tuna.tsinghua.edu.cn
  info "切换 apt 软件源为清华镜像（原配置备份为 *.ss-bak）"
  for f in /etc/apt/sources.list /etc/apt/sources.list.d/ubuntu.sources /etc/apt/sources.list.d/debian.sources; do
    [[ -f $f ]] || continue
    [[ -f $f.ss-bak ]] || cp -a "$f" "$f.ss-bak"
    sed -i -E \
      -e "s#https?://(archive|security)\.ubuntu\.com/ubuntu/?#$m/ubuntu/#g" \
      -e "s#https?://ports\.ubuntu\.com/ubuntu-ports/?#$m/ubuntu-ports/#g" \
      -e "s#https?://deb\.debian\.org/debian-security/?#$m/debian-security/#g" \
      -e "s#https?://(deb|security)\.debian\.org/debian/?#$m/debian/#g" "$f"
  done
}

APT_OPTS=(-q -o APT::Status-Fd=3 -o DPkg::Lock::Timeout=900 -o Dpkg::Options::=--force-confold)

apt_install() {
  local missing=() p
  for p in "$@"; do
    [[ $(dpkg-query -W -f='${Status}' "$p" 2>/dev/null || true) == "install ok installed" ]] || missing+=("$p")
  done
  if [[ ${#missing[@]} -eq 0 ]]; then ok "依赖软件包均已安装"; return; fi
  info "需要安装 ${#missing[@]} 个软件包：${missing[*]}"
  run_progress "软件包安装完成" env DEBIAN_FRONTEND=noninteractive apt-get install -y "${APT_OPTS[@]}" "${missing[@]}" \
    || { tail -n 20 "$SS_LOG" >&2; die "安装软件包失败：${missing[*]}（完整日志 $SS_LOG）"; }
}

install_deps() {
  info "安装依赖软件（nginx / redis / ffmpeg / 数据库 等，首次安装可能需要几分钟）"
  if [[ "${SS_APT_MIRROR:-}" == "tuna" ]]; then switch_apt_mirror; fi
  : >"$SS_LOG"
  # 上次安装被中断时 dpkg 可能处于未配置状态，先修复
  if [[ -n $(dpkg --audit 2>/dev/null || true) ]]; then
    run_progress "修复上次未完成的软件包配置" env DEBIAN_FRONTEND=noninteractive dpkg --configure -a || true
  fi
  run_progress "软件源索引已更新" env DEBIAN_FRONTEND=noninteractive apt-get update "${APT_OPTS[@]}" \
    || { tail -n 20 "$SS_LOG" >&2; die "apt-get update 失败（可设置 SS_APT_MIRROR=tuna 使用国内镜像后重试）"; }
  local pkgs=(curl ca-certificates tar gzip openssl iproute2 logrotate nginx redis-server ffmpeg)
  # 数据库：已有 MySQL/MariaDB 则复用，否则 Ubuntu 装 mysql-server、Debian 装 mariadb-server
  if command -v mysqld >/dev/null || command -v mariadbd >/dev/null; then
    ok "检测到已安装的 MySQL/MariaDB，直接复用"
  elif [[ $OS_ID == debian ]]; then pkgs+=(mariadb-server)
  else pkgs+=(mysql-server); fi
  apt_install "${pkgs[@]}"
  # ZLMediaKit 依赖 OpenSSL 3
  [[ $(ldconfig -p 2>/dev/null || true) == *libssl.so.3* ]] || apt_install libssl3 || die "缺少 libssl3（系统过旧），请升级到 Ubuntu 22.04+/Debian 12+"
  DB_SERVICE=
  local u; for u in mariadb mysql; do
    [[ -n $(systemctl list-unit-files "$u.service" --no-legend 2>/dev/null || true) ]] && { DB_SERVICE=$u; break; }
  done
  [[ -n $DB_SERVICE ]] || DB_SERVICE=mysql
  systemctl enable --now "$DB_SERVICE" redis-server nginx >/dev/null 2>&1 || true
  for s in "$DB_SERVICE" redis-server nginx; do
    systemctl is-active --quiet "$s" || systemctl start "$s" || die "服务 $s 启动失败（journalctl -u $s -n 50）"
  done
  ok "依赖已就绪：$(nginx -v 2>&1 | sed 's#nginx version: ##')，$(redis-server --version | awk '{print "redis " $3}' | tr -d 'v='), $(mysql --version 2>/dev/null | grep -oE '(MariaDB|Distrib [0-9.]+|Ver [0-9.]+)' | head -1)，ffmpeg $(ffmpeg -version 2>/dev/null | awk 'NR==1{print $3}')"
}

# ----------------------------------------------------------------------------- 安装包
PKG_DIR=
cleanup_pkg() { if [[ -n ${PKG_TMP:-} && -d $PKG_TMP ]]; then rm -rf "$PKG_TMP"; fi; return 0; }
trap cleanup_pkg EXIT

fetch_package() {
  # 1) 脚本所在目录就是解压后的发布包
  if [[ -x $SCRIPT_DIR/bin/livems && -x $SCRIPT_DIR/bin/MediaServer && -f $SCRIPT_DIR/VERSION && $SCRIPT_DIR != "$INSTALL_DIR" ]]; then
    PKG_DIR=$SCRIPT_DIR; ok "使用本地安装包：$PKG_DIR"; return
  fi
  PKG_TMP=$(mktemp -d /tmp/streamswitch-pkg.XXXXXX)
  local src=${SS_PACKAGE:-} url
  if [[ -n $src && -f $src ]]; then
    info "解压本地安装包 $src"
    tar -xzf "$src" -C "$PKG_TMP"
  else
    if [[ -n $src ]]; then url=$src
    elif [[ -n ${SS_VERSION:-} ]]; then url="https://github.com/$REPO/releases/download/$SS_VERSION/streamswitch-linux-$ARCH.tar.gz"
    else url="https://github.com/$REPO/releases/latest/download/streamswitch-linux-$ARCH.tar.gz"; fi
    local proxy=${GH_PROXY:-}
    while true; do
      info "下载安装包（$ARCH）：${proxy}$url"
      local cprog=(-sS); [[ -t 2 ]] && cprog=(--progress-bar)
      if curl -fL "${cprog[@]}" --retry 3 --connect-timeout 15 -o "$PKG_TMP/pkg.tar.gz" "${proxy}$url"; then break; fi
      warn "下载失败"
      interactive || die "无法下载安装包，可设置 GH_PROXY=<GitHub 加速前缀> 或 SS_PACKAGE=<本地安装包路径> 后重试"
      proxy=$(ask "可输入 GitHub 加速前缀重试（如 https://ghfast.top/ ，留空退出）" "")
      [[ -n $proxy ]] || die "已取消"
    done
    if curl -fsL --connect-timeout 10 -o "$PKG_TMP/pkg.sha256" "${proxy}$url.sha256" 2>/dev/null; then
      (cd "$PKG_TMP" && echo "$(awk '{print $1}' pkg.sha256)  pkg.tar.gz" | sha256sum -c --quiet -) || die "安装包校验失败（sha256 不匹配），请重试"
      ok "安装包 sha256 校验通过"
    fi
    tar -xzf "$PKG_TMP/pkg.tar.gz" -C "$PKG_TMP"
  fi
  PKG_DIR=$(find "$PKG_TMP" -maxdepth 2 -name VERSION -printf '%h\n' 2>/dev/null | sort | head -1 || true)
  [[ -n $PKG_DIR && -x $PKG_DIR/bin/livems && -x $PKG_DIR/bin/MediaServer ]] || die "安装包内容不完整"
  local parch; parch=$(cat "$PKG_DIR/ARCH" 2>/dev/null || echo "$ARCH")
  [[ $parch == "$ARCH" ]] || die "安装包架构（$parch）与本机（$ARCH）不一致"
}

pkg_version() { cat "$PKG_DIR/VERSION"; }
cur_version() { cat "$INSTALL_DIR/VERSION" 2>/dev/null || echo "未知"; }

# ----------------------------------------------------------------------------- 数据库 / Redis
mysql_root() {
  if [[ -n ${MYSQL_ROOT_PASS:-} ]]; then MYSQL_PWD=$MYSQL_ROOT_PASS mysql -uroot -h127.0.0.1 "$@"
  else mysql -uroot "$@"; fi
}

dump_db() {
  MYSQL_PWD=$DB_PASS mysqldump -h"$DB_HOST" -P"$DB_PORT" -u"$DB_USER" --single-transaction --no-tablespaces "$DB_NAME" | gzip >"$1"
}

ensure_mysql_root() {
  # 依次尝试：已知密码 → root 免密（auth_socket/unix_socket）→ SS_MYSQL_ROOT_PASSWORD → 人工输入
  if [[ -n ${MYSQL_ROOT_PASS:-} ]] && mysql_root -e "SELECT 1" >/dev/null 2>&1; then return; fi
  MYSQL_ROOT_PASS=
  mysql_root -e "SELECT 1" >/dev/null 2>&1 && return
  MYSQL_ROOT_PASS=${SS_MYSQL_ROOT_PASSWORD:-}
  local tries=0
  until [[ -n $MYSQL_ROOT_PASS ]] && mysql_root -e "SELECT 1" >/dev/null 2>&1; do
    tries=$((tries + 1))
    [[ $tries -gt 3 ]] && die "无法以 root 身份连接 MySQL/MariaDB"
    interactive || die "无法以 root 免密连接数据库，请通过 SS_MYSQL_ROOT_PASSWORD 提供 root 密码"
    MYSQL_ROOT_PASS=$(ask_secret "请输入本机 MySQL/MariaDB 的 root 密码")
  done
}

setup_mysql() {
  info "配置数据库 $DB_NAME"
  ensure_mysql_root
  mysql_root <<SQL
CREATE DATABASE IF NOT EXISTS \`$DB_NAME\` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS '$DB_USER'@'127.0.0.1' IDENTIFIED BY '$DB_PASS';
CREATE USER IF NOT EXISTS '$DB_USER'@'localhost' IDENTIFIED BY '$DB_PASS';
ALTER USER '$DB_USER'@'127.0.0.1' IDENTIFIED BY '$DB_PASS';
ALTER USER '$DB_USER'@'localhost' IDENTIFIED BY '$DB_PASS';
GRANT ALL PRIVILEGES ON \`$DB_NAME\`.* TO '$DB_USER'@'127.0.0.1';
GRANT ALL PRIVILEGES ON \`$DB_NAME\`.* TO '$DB_USER'@'localhost';
FLUSH PRIVILEGES;
SQL
  MYSQL_PWD=$DB_PASS mysql -h"$DB_HOST" -P"$DB_PORT" -u"$DB_USER" "$DB_NAME" -e "SELECT 1" >/dev/null \
    || die "应用数据库账号验证失败"
  ok "数据库已就绪（库/账号：$DB_NAME / $DB_USER）"
}

rcli() {
  if [[ -n ${REDIS_PASS:-} ]]; then REDISCLI_AUTH=$REDIS_PASS redis-cli -h "${REDIS_HOST:-127.0.0.1}" -p "${REDIS_PORT:-6379}" "$@"
  else redis-cli -h "${REDIS_HOST:-127.0.0.1}" -p "${REDIS_PORT:-6379}" "$@"; fi
}

setup_redis() {
  REDIS_PASS=${SS_REDIS_PASSWORD:-${REDIS_PASS:-}}
  local r
  while true; do
    r=$(rcli ping 2>&1 || true)
    [[ $r == PONG ]] && { ok "Redis 已就绪"; return; }
    if [[ $r == *NOAUTH* || $r == *WRONGPASS* || $r == *"invalid password"* ]]; then
      interactive || die "Redis 需要密码，请通过 SS_REDIS_PASSWORD 提供"
      REDIS_PASS=$(ask_secret "本机 Redis 设置了密码，请输入")
    else
      die "无法连接 Redis：$r"
    fi
  done
}

# ----------------------------------------------------------------------------- 生成配置
ini_set() { # 文件 节 键 值
  awk -v s="[$2]" -v k="$3" -v v="$4" '
    BEGIN { insec = 0; done = 0 }
    /^\[/ { if (insec && !done) { print k "=" v; done = 1 } insec = ($0 == s) }
    insec && index($0, k "=") == 1 { if (!done) { print k "=" v; done = 1 } next }
    { print }
    END { if (!done) { if (!insec) print s; print k "=" v } }' "$1" >"$1.tmp" && mv "$1.tmp" "$1"
}

write_zlm_conf() {
  local f=$INSTALL_DIR/zlm/conf/config.ini hook="http://127.0.0.1:$APP_PORT/index/hook"
  cp "$PKG_DIR/templates/zlm.ini" "$f"
  ini_set "$f" api secret "$ZLM_SECRET"
  ini_set "$f" general mediaServerId streamswitch
  [[ -f /proc/net/if_inet6 ]] || ini_set "$f" general listen_ip 0.0.0.0   # 无 IPv6 的主机只监听 IPv4
  ini_set "$f" http port "$ZLM_HTTP_PORT"
  ini_set "$f" http rootPath "$INSTALL_DIR/zlm/www"
  ini_set "$f" protocol hls_save_path "$INSTALL_DIR/zlm/www"
  ini_set "$f" rtmp port "$RTMP_PORT"
  ini_set "$f" rtsp port "$RTSP_PORT"
  ini_set "$f" rtp port 0          # 未使用的协议端口全部关闭，减少端口冲突与暴露面
  ini_set "$f" rtp_proxy port 0
  ini_set "$f" srt port 0
  ini_set "$f" rtc udpPort 0
  ini_set "$f" rtc tcpPort 0
  ini_set "$f" ffmpeg bin /usr/bin/ffmpeg
  for h in on_publish on_play on_stream_changed on_flow_report on_stream_not_found on_stream_none_reader on_server_started on_server_exited; do
    ini_set "$f" hook "$h" "$hook/$h"
  done
  chmod 600 "$f"
}

write_app_conf() {
  local f=$INSTALL_DIR/backend/configs/config.yaml
  umask 077
  cat >"$f" <<EOF
# 由 StreamSwitch 部署脚本生成。端口请用 streamswitch 菜单修改；升级时会重新生成（旧文件备份在 backup/ 目录）。
server:
  host: 127.0.0.1
  port: $APP_PORT
  mode: release
  trusted_proxies: ["127.0.0.1", "::1"]
  read_timeout: 30
  write_timeout: 30
  shutdown_timeout: 10
  static_dir: ""
  pprof_addr: ""
mysql:
  host: $DB_HOST
  port: $DB_PORT
  user: $DB_USER
  password: "$DB_PASS"
  database: $DB_NAME
  charset: utf8mb4
  max_open_conns: 50
  max_idle_conns: 10
  conn_max_lifetime: 3600
  parse_time: true
redis:
  host: $REDIS_HOST
  port: $REDIS_PORT
  password: "$REDIS_PASS"
  db: 0
  pool_size: 20
  dial_timeout: 5
  read_timeout: 3
  write_timeout: 3
  prefix: "streamswitch:"
jwt:
  secret: "$JWT_SECRET"
  issuer: streamswitch
  expire_hour: 24
zlm:
  base_url: "http://127.0.0.1:$ZLM_HTTP_PORT"
  secret: "$ZLM_SECRET"
  app: live
  vhost: "__defaultVhost__"
  timeout: 10
  hook_path: /index/hook
  sync_secs: 30
  rtmp_port: $RTMP_PORT
  rtmps_port: 0
hls:
  upstream_url: "http://127.0.0.1:$ZLM_HTTP_PORT"
  zlm_www_dir: "$INSTALL_DIR/zlm/www"
  key_dir: "$INSTALL_DIR/backend/data/hls_key"
  cache_dir: "$INSTALL_DIR/backend/data/hls_cache"
  default_expire_sec: 0
  iv_strategy: seq
auth:
  enabled: true
  token_expire: 3600
  ip_bind_enabled: false
  param_name: token
  allow_empty: false
  allow_anonymous: false
log:
  level: info
  format: text
  output_file: ""
frontend:
  dist_dir: "$INSTALL_DIR/frontend/dist"
nginx:
  vhost_conf: "$NGX_CONF"
  cert_dir: "$INSTALL_DIR/certs"
  reload_cmd: "nginx -s reload"
studio:
  ffmpeg_path: "/usr/bin/ffmpeg"
  ffprobe_path: "/usr/bin/ffprobe"
  font_file: "$INSTALL_DIR/backend/assets/fonts/NotoSansSC-Regular.otf"
  data_dir: "$INSTALL_DIR/backend/data/studio"
  push_base: "rtmp://127.0.0.1:$RTMP_PORT"
EOF
  umask 022
}

ensure_cert() {
  local d=$INSTALL_DIR/certs/selfsigned
  [[ -f $d/fullchain.pem && -f $d/privkey.pem ]] && return
  mkdir -p "$d"
  openssl req -x509 -newkey rsa:2048 -nodes -days 3650 -subj "/CN=StreamSwitch" \
    -addext "subjectAltName=IP:$(lan_ip),IP:127.0.0.1,DNS:localhost" \
    -keyout "$d/privkey.pem" -out "$d/fullchain.pem" >/dev/null 2>&1
  chmod 700 "$d"; chmod 600 "$d/privkey.pem"
}

write_nginx_conf() {
  # 保留后台「域名配置 → 应用证书」写入的证书路径；没有则用自签名占位证书
  local cert="$INSTALL_DIR/certs/selfsigned/fullchain.pem" key="$INSTALL_DIR/certs/selfsigned/privkey.pem" v6=""
  if [[ -f $NGX_CONF ]]; then
    local c k
    c=$(awk '$1=="ssl_certificate"{sub(";","",$2); print $2; exit}' "$NGX_CONF")
    k=$(awk '$1=="ssl_certificate_key"{sub(";","",$2); print $2; exit}' "$NGX_CONF")
    [[ -f $c && -f $k ]] && cert=$c && key=$k
  fi
  [[ -f /proc/net/if_inet6 ]] && v6=1
  local proxy_common="proxy_set_header Host \$host;
        proxy_set_header X-Forwarded-Host \$http_host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;"
  cat >"$NGX_CONF" <<EOF
# 由 StreamSwitch 部署脚本生成。端口请用 streamswitch 菜单修改；SSL 证书请在后台「域名配置」上传并应用。
server {
    listen $HTTP_PORT;
${v6:+    listen [::]:$HTTP_PORT;}
    listen $HTTPS_PORT ssl http2;
${v6:+    listen [::]:$HTTPS_PORT ssl http2;}
    server_name _;
    client_max_body_size 4096m;

    ssl_certificate     $cert;
    ssl_certificate_key $key;
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_session_cache shared:STREAMSWITCH_SSL:10m;
    ssl_session_timeout 10m;
    error_page 497 https://\$host:\$server_port\$request_uri;

    # 后台 API（素材上传可达 4GB：流式转发 + 放宽超时）
    location /api/ {
        proxy_pass http://127.0.0.1:$APP_PORT;
        proxy_http_version 1.1;
        proxy_request_buffering off;
        # 不缓冲请求体时，HTTP/2 的上传窗口取 max(本值, 64k)；默认值下高延迟链路（经代理/跨境）
        # 上传被限制在约 64KB/RTT（RTT 170ms 时仅 0.4MB/s）。8m 只在有请求体时按需分配。
        client_body_buffer_size 8m;
        proxy_read_timeout 1800s;
        proxy_send_timeout 1800s;
        $proxy_common
    }
    # HLS 加密播放 / 密钥下发
    location /hls-enc/ {
        proxy_pass http://127.0.0.1:$APP_PORT;
        proxy_http_version 1.1;
        proxy_buffering off;
        proxy_read_timeout 3600s;
        expires -1;
        $proxy_common
    }
    location /hls-key/ {
        proxy_pass http://127.0.0.1:$APP_PORT;
        $proxy_common
    }
    # 播放分发（HTTP-FLV / WS-FLV / HLS）→ ZLMediaKit
    location /live/ {
        proxy_pass http://127.0.0.1:$ZLM_HTTP_PORT;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_buffering off;
        proxy_read_timeout 3600s;
        expires -1;
        $proxy_common
    }
    # 前端页面
    location / {
        proxy_pass http://127.0.0.1:$APP_PORT;
        $proxy_common
    }

    access_log /var/log/nginx/streamswitch.access.log;
    error_log  /var/log/nginx/streamswitch.error.log;
}
EOF
  sed -i '/^$/N;/^\n$/D' "$NGX_CONF"
  nginx -t >/tmp/streamswitch-nginx.log 2>&1 || { cat /tmp/streamswitch-nginx.log >&2; die "Nginx 配置校验失败"; }
}

write_systemd() {
  cat >/etc/systemd/system/$SVC_ZLM.service <<EOF
[Unit]
Description=StreamSwitch 流媒体内核 (ZLMediaKit)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR/zlm
ExecStart=$INSTALL_DIR/zlm/bin/MediaServer -c $INSTALL_DIR/zlm/conf/config.ini
Restart=always
RestartSec=3
LimitNOFILE=65535
StandardOutput=null
StandardError=journal

[Install]
WantedBy=multi-user.target
EOF
  cat >/etc/systemd/system/$SVC_APP.service <<EOF
[Unit]
Description=StreamSwitch 流媒体管理系统
After=network-online.target $DB_SERVICE.service redis-server.service $SVC_ZLM.service
Wants=network-online.target $SVC_ZLM.service

[Service]
Type=simple
WorkingDirectory=$INSTALL_DIR/backend
ExecStart=$INSTALL_DIR/backend/bin/livems -c $INSTALL_DIR/backend/configs/config.yaml
Restart=always
RestartSec=3
LimitNOFILE=65535
KillSignal=SIGTERM
TimeoutStopSec=30
StandardOutput=append:$INSTALL_DIR/backend/logs/livems.log
StandardError=append:$INSTALL_DIR/backend/logs/livems.log

[Install]
WantedBy=multi-user.target
EOF
  cat >"$LOGROTATE" <<EOF
$INSTALL_DIR/backend/logs/*.log {
    daily
    rotate 7
    compress
    missingok
    notifempty
    copytruncate
}
EOF
  systemctl daemon-reload
  systemctl enable "$SVC_ZLM" "$SVC_APP" >/dev/null 2>&1
}

open_firewall() {
  command -v ufw >/dev/null || return 0
  [[ $(ufw status 2>/dev/null || true) == *"Status: active"* ]] || return 0
  info "检测到 ufw 防火墙已启用，放行对外端口"
  local p
  for p in $HTTP_PORT $HTTPS_PORT $RTMP_PORT; do ufw allow "$p/tcp" >/dev/null; done
  [[ $RTSP_PORT != 0 ]] && ufw allow "$RTSP_PORT/tcp" >/dev/null
  ok "ufw 已放行 $HTTP_PORT、$HTTPS_PORT、$RTMP_PORT$(rtsp_txt)"
}

copy_program_files() {
  install -d -m 755 "$INSTALL_DIR"/{backend/bin,backend/configs,backend/assets/fonts,backend/logs,frontend,zlm/bin,zlm/conf,zlm/www,zlm/log,certs,backup}
  install -d -m 750 "$INSTALL_DIR"/backend/data/{studio/videos,studio/logos,hls_key,hls_cache}
  install -m 755 "$PKG_DIR/bin/livems" "$INSTALL_DIR/backend/bin/livems"
  install -m 755 "$PKG_DIR/bin/MediaServer" "$INSTALL_DIR/zlm/bin/MediaServer"
  rm -rf "$INSTALL_DIR/frontend/dist.new" && cp -a "$PKG_DIR/web" "$INSTALL_DIR/frontend/dist.new"
  rm -rf "$INSTALL_DIR/frontend/dist" && mv "$INSTALL_DIR/frontend/dist.new" "$INSTALL_DIR/frontend/dist"
  cp -a "$PKG_DIR/assets/fonts/." "$INSTALL_DIR/backend/assets/fonts/"
  install -d -m 755 "$INSTALL_DIR/templates"
  cp "$PKG_DIR/templates/zlm.ini" "$INSTALL_DIR/templates/zlm.ini"
  # 用「写临时文件 + rename」替换脚本：通过 streamswitch 命令升级时，正在执行的旧脚本不受影响
  install -m 755 "$PKG_DIR/install.sh" "$INSTALL_DIR/.install.sh.new" && mv -f "$INSTALL_DIR/.install.sh.new" "$INSTALL_DIR/install.sh"
  cp "$PKG_DIR/VERSION" "$INSTALL_DIR/VERSION"
  ln -sf "$INSTALL_DIR/install.sh" "$BIN_LINK"
}

# 全新安装时：发现上次卸载保留的素材/证书，询问是否恢复
restore_kept_data() {
  local d; d=$(ls -1dt "${INSTALL_DIR}"-data-* 2>/dev/null | head -1 || true)
  [[ -n $d && -d $d ]] || return 0
  ask_yes "检测到上次卸载保留的数据 $d（素材/证书），是否恢复" y || return 0
  [[ -d $d/data ]] && cp -a "$d/data/." "$INSTALL_DIR/backend/data/"
  [[ -d $d/certs ]] && cp -a "$d/certs/." "$INSTALL_DIR/certs/"
  ok "已恢复素材与证书（数据库若仍保留在本机会自动沿用）"
}

start_services() {
  local spin_chars='|/-\'
  info "启动服务"
  systemctl restart "$SVC_ZLM"
  sleep 1
  systemctl restart "$SVC_APP"
  local i
  for i in $(seq 1 40); do
    curl -fsS --max-time 2 "http://127.0.0.1:$APP_PORT/health" >/dev/null 2>&1 && break
    [[ -t 1 ]] && printf '\r  %s 等待后端就绪… %ds\033[K' "${spin_chars:i%4:1}" "$i"
    sleep 1
  done
  [[ -t 1 ]] && printf '\r\033[K'
  if ! curl -fsS --max-time 2 "http://127.0.0.1:$APP_PORT/health" >/dev/null 2>&1; then
    tail -n 20 "$INSTALL_DIR/backend/logs/livems.log" >&2 || true
    die "后端未能正常启动（日志：$INSTALL_DIR/backend/logs/livems.log）"
  fi
  systemctl is-active --quiet "$SVC_ZLM" || { journalctl -u "$SVC_ZLM" -n 20 --no-pager >&2; die "ZLMediaKit 启动失败"; }
  systemctl reload nginx 2>/dev/null || systemctl restart nginx
  curl -fsS --max-time 3 "http://127.0.0.1:$HTTP_PORT/health" >/dev/null || die "Nginx 未能转发到后端（端口 $HTTP_PORT）"
  ok "服务运行正常：$SVC_ZLM、$SVC_APP、nginx"
}

print_summary() {
  local ip pub need
  ip=$(lan_ip)
  pub=$(curl -4 -fsS --max-time 4 https://api.ipify.org 2>/dev/null || true)
  need=$(curl -fsS --max-time 3 "http://127.0.0.1:$APP_PORT/api/v1/setup/status" 2>/dev/null | grep -o '"need_setup":true' || true)
  echo
  echo "${C_G}${C_W}================================================================${C_0}"
  echo "${C_G}${C_W}  StreamSwitch $(cur_version) 部署完成${C_0}"
  echo "${C_G}${C_W}================================================================${C_0}"
  echo "  后台地址：     ${C_W}http://$ip:$HTTP_PORT/${C_0}"
  echo "  HTTPS 地址：   https://$ip:$HTTPS_PORT/   （当前为自签名证书，浏览器会提示不安全）"
  [[ -n $pub ]] && echo "  公网访问：     http://$pub:$HTTP_PORT/   （需在路由器映射端口；该 IP 为检测值，经代理出口时可能不准）"
  echo "  推流地址：     rtmp://$ip:$RTMP_PORT/live/<流ID>?key=<推流Key>   （在后台「流管理」创建流后获取）"
  [[ $RTSP_PORT != 0 ]] && echo "  RTSP 端口：    $RTSP_PORT"
  echo
  if [[ -n $need ]]; then
    echo "  ${C_Y}${C_W}▶ 首次打开后台会要求注册账号：第一个注册的账号即为超级管理员。${C_0}"
  fi
  echo "  ▶ 域名与 SSL 证书：登录后在「域名配置」中上传证书并填写推流/播放域名（不填则自动使用访问地址）。"
  echo "  ▶ 路由器端口映射（外网访问时）：$HTTP_PORT、$HTTPS_PORT、$RTMP_PORT$(rtsp_txt)（TCP）"
  echo "  ▶ 管理命令：sudo streamswitch    （升级 / 改端口 / 卸载 / 重置密码）"
  echo "  ▶ 日志：$INSTALL_DIR/backend/logs/livems.log    journalctl -u $SVC_ZLM"
  echo
}

# ----------------------------------------------------------------------------- 1. 安装 / 升级
do_install() {
  require_root; detect_os; detect_arch
  if installed; then do_upgrade; return; fi

  echo
  info "全新安装 StreamSwitch（系统：$OS_NAME，架构：$ARCH，安装目录：$INSTALL_DIR）"
  if [[ -z ${SS_APT_MIRROR:-} ]] && ask_yes "是否将 apt 软件源切换为国内镜像（清华 TUNA）以加速下载" n; then SS_APT_MIRROR=tuna; fi
  install_deps
  fetch_package
  info "安装版本：$(pkg_version)"

  echo
  info "端口设置（直接回车使用默认值）"
  HTTP_PORT=$(pick_port "后台 HTTP 端口" 51880 SS_HTTP_PORT)
  HTTPS_PORT=$(pick_port "后台 HTTPS 端口" 51443 SS_HTTPS_PORT)
  RTMP_PORT=$(pick_port "RTMP 推流/播放端口" 1935 SS_RTMP_PORT)
  RTSP_PORT=$(pick_port "RTSP 端口（填 0 关闭）" 554 SS_RTSP_PORT "" 1)
  APP_PORT=$(free_port 18090)
  ZLM_HTTP_PORT=$(free_port 18080)
  [[ $ZLM_HTTP_PORT == "$APP_PORT" ]] && ZLM_HTTP_PORT=$(free_port $((APP_PORT + 1)))

  DB_HOST=127.0.0.1; DB_PORT=3306; DB_NAME=streamswitch; DB_USER=streamswitch; DB_PASS=$(rand_hex 16)
  REDIS_HOST=127.0.0.1; REDIS_PORT=6379; REDIS_PASS=
  ZLM_SECRET=$(rand_hex 16); JWT_SECRET=$(rand_hex 24)
  setup_mysql
  setup_redis

  info "部署程序文件到 $INSTALL_DIR"
  copy_program_files
  restore_kept_data
  save_env
  write_app_conf
  write_zlm_conf
  ensure_cert
  write_nginx_conf
  write_systemd
  "$INSTALL_DIR/backend/bin/livems" -c "$INSTALL_DIR/backend/configs/config.yaml" -migrate >/dev/null || die "数据库初始化失败"
  ok "数据库表结构已初始化"
  open_firewall
  start_services
  print_summary
}

do_upgrade() {
  load_env
  local old new ts bk
  old=$(cur_version)
  info "检测到已安装 StreamSwitch $old，执行升级"
  install_deps
  fetch_package
  new=$(pkg_version)
  if [[ $new == "$old" && ${SS_FORCE:-} != 1 ]]; then
    ask_yes "当前已是 $new，是否重新安装（覆盖程序文件，保留数据与配置）" n || { ok "无需升级"; return; }
  fi
  ts=$(date +%Y%m%d-%H%M%S); bk=$INSTALL_DIR/backup/$ts
  info "备份当前版本到 $bk"
  mkdir -p "$bk"
  cp -a "$INSTALL_DIR/backend/bin/livems" "$INSTALL_DIR/zlm/bin/MediaServer" "$INSTALL_DIR/VERSION" "$bk/" 2>/dev/null || true
  cp -a "$INSTALL_DIR/backend/configs/config.yaml" "$INSTALL_DIR/zlm/conf/config.ini" "$NGX_CONF" "$bk/" 2>/dev/null || true
  tar -czf "$bk/frontend.tar.gz" -C "$INSTALL_DIR/frontend" dist 2>/dev/null || true
  if command -v mysqldump >/dev/null; then
    run_progress "数据库已备份：$bk/database.sql.gz" dump_db "$bk/database.sql.gz" || warn "数据库备份失败（继续升级）"
  fi
  ls -1dt "$INSTALL_DIR"/backup/*/ 2>/dev/null | tail -n +4 | xargs -r rm -rf   # 只保留最近 3 份

  setup_redis
  systemctl stop "$SVC_APP" "$SVC_ZLM" 2>/dev/null || true
  copy_program_files
  save_env
  write_app_conf
  write_zlm_conf
  ensure_cert
  write_nginx_conf
  write_systemd
  "$INSTALL_DIR/backend/bin/livems" -c "$INSTALL_DIR/backend/configs/config.yaml" -migrate >/dev/null || die "数据库升级失败（可从 $bk 恢复）"
  ok "数据库结构已升级"
  start_services
  ok "已从 $old 升级到 $new（备份：$bk）"
  print_summary
}

# ----------------------------------------------------------------------------- 2. 端口设置
do_port() {
  require_root; load_env
  echo
  info "当前端口：HTTP $HTTP_PORT ｜ HTTPS $HTTPS_PORT ｜ RTMP $RTMP_PORT ｜ RTSP $([[ $RTSP_PORT == 0 ]] && echo 已关闭 || echo "$RTSP_PORT")"
  info "输入新端口，直接回车保持不变"
  local oh=$HTTP_PORT os=$HTTPS_PORT orr=$RTMP_PORT ors=$RTSP_PORT
  HTTP_PORT=$(pick_port "后台 HTTP 端口" "$oh" SS_HTTP_PORT "$oh")
  HTTPS_PORT=$(pick_port "后台 HTTPS 端口" "$os" SS_HTTPS_PORT "$os")
  RTMP_PORT=$(pick_port "RTMP 端口" "$orr" SS_RTMP_PORT "$orr")
  RTSP_PORT=$(pick_port "RTSP 端口（填 0 关闭）" "$ors" SS_RTSP_PORT "$ors" 1)
  if [[ "$HTTP_PORT $HTTPS_PORT $RTMP_PORT $RTSP_PORT" == "$oh $os $orr $ors" ]]; then ok "端口未变化"; return; fi
  local dup; dup=$(printf '%s\n' "$HTTP_PORT" "$HTTPS_PORT" "$RTMP_PORT" "$RTSP_PORT" | grep -v '^0$' | sort | uniq -d)
  [[ -z $dup ]] || die "端口不能重复：$dup"
  PKG_DIR=$INSTALL_DIR
  [[ -f $INSTALL_DIR/templates/zlm.ini ]] || { mkdir -p "$INSTALL_DIR/templates"; cp "$INSTALL_DIR/zlm/conf/config.ini" "$INSTALL_DIR/templates/zlm.ini"; }
  save_env
  write_app_conf
  write_zlm_conf
  write_nginx_conf
  # 后台「对外接入设置」中保存过推流端口的，同步为新 RTMP 端口
  MYSQL_PWD=$DB_PASS mysql -h"$DB_HOST" -P"$DB_PORT" -u"$DB_USER" "$DB_NAME" -e \
    "UPDATE sys_config SET config_val=JSON_SET(config_val,'\$.push_port',$RTMP_PORT) WHERE config_key='ingress' AND JSON_VALID(config_val)" 2>/dev/null || true
  open_firewall
  start_services
  # 确认旧端口已释放：nginx 偶尔保留旧监听，必要时完整重启
  local p
  for p in "$oh" "$os"; do
    [[ $p == "$HTTP_PORT" || $p == "$HTTPS_PORT" ]] && continue
    if ! wait_port_free "$p"; then systemctl restart nginx; wait_port_free "$p" || warn "旧端口 $p 仍在监听，请检查是否被其他程序占用"; fi
  done
  for p in "$orr" "$ors"; do
    [[ $p == 0 || $p == "$RTMP_PORT" || $p == "$RTSP_PORT" ]] && continue
    wait_port_free "$p" || warn "旧端口 $p 仍在监听，请检查是否被其他程序占用"
  done
  ok "端口已更新"
  warn "如已在路由器做了端口映射、或在后台「域名配置」中填写过端口，请同步修改"
  print_summary
}

# ----------------------------------------------------------------------------- 3. 卸载
do_uninstall() {
  require_root
  installed || [[ -d $INSTALL_DIR || -f /etc/systemd/system/$SVC_APP.service ]] || { ok "未检测到 StreamSwitch 安装"; return; }
  [[ -f $ENV_FILE ]] && { set -a; . "$ENV_FILE"; set +a; }
  echo
  warn "即将卸载 StreamSwitch（$INSTALL_DIR）"
  # 交互时默认「否」；无人值守需显式 SS_YES=1 才执行
  [[ ${SS_YES:-} == 1 ]] || ask_yes "确认卸载" n || { ok "已取消"; return; }
  local purge=n
  if [[ ${SS_PURGE:-} == 1 ]]; then purge=y
  elif ask_yes "是否同时删除数据库与全部素材数据（不可恢复）" n; then purge=y; fi

  systemctl disable --now "$SVC_APP" "$SVC_ZLM" >/dev/null 2>&1 || true
  rm -f /etc/systemd/system/$SVC_APP.service /etc/systemd/system/$SVC_ZLM.service "$LOGROTATE" "$BIN_LINK"
  systemctl daemon-reload
  rm -f "$NGX_CONF"
  nginx -t >/dev/null 2>&1 && systemctl reload nginx 2>/dev/null || true

  if [[ $purge == y ]]; then
    if [[ -n ${DB_NAME:-} ]]; then
      ( ensure_mysql_root && mysql_root -e "DROP DATABASE IF EXISTS \`$DB_NAME\`; DROP USER IF EXISTS '$DB_USER'@'127.0.0.1'; DROP USER IF EXISTS '$DB_USER'@'localhost';" ) \
        && ok "数据库 $DB_NAME 已删除" || warn "数据库删除失败，请手动执行：DROP DATABASE $DB_NAME"
      rcli --scan --pattern 'streamswitch:*' 2>/dev/null | while read -r k; do rcli del "$k" >/dev/null; done || true
    fi
    rm -rf "$INSTALL_DIR"
    ok "已卸载并删除全部数据"
  else
    local keep=${INSTALL_DIR}-data-$(date +%Y%m%d-%H%M%S)
    mkdir -p "$keep"
    [[ -d $INSTALL_DIR/backend/data ]] && mv "$INSTALL_DIR/backend/data" "$keep/data"
    [[ -d $INSTALL_DIR/certs ]] && mv "$INSTALL_DIR/certs" "$keep/certs"
    [[ -f $ENV_FILE ]] && mv "$ENV_FILE" "$keep/install.env"
    if [[ -n ${DB_NAME:-} ]] && command -v mysqldump >/dev/null; then
      run_progress "数据库已备份到 $keep/database.sql.gz" dump_db "$keep/database.sql.gz" || true
    fi
    rm -rf "$INSTALL_DIR"
    ok "已卸载程序。数据库 ${DB_NAME:-streamswitch} 保留在本机；素材、证书与数据库备份已移至 $keep"
  fi
  echo "  （依赖软件 nginx / redis / mysql / ffmpeg 未卸载，如不再需要可自行 apt remove）"
}

# ----------------------------------------------------------------------------- 4. 重置管理员密码
do_reset_password() {
  require_root; load_env
  local bin=$INSTALL_DIR/backend/bin/livems conf=$INSTALL_DIR/backend/configs/config.yaml users target pwd pwd2 gen=
  users=$("$bin" -c "$conf" -list-users) || die "读取用户列表失败"
  if [[ -z $users ]]; then
    warn "系统中还没有任何用户：请打开后台 http://$(lan_ip):$HTTP_PORT/ 注册，第一个注册的账号即为超级管理员"
    return
  fi
  echo
  printf "  %-5s %-20s %-16s %-6s %s\n" ID 用户名 昵称 超管 状态
  awk -F'\t' '{printf "  %-5s %-20s %-16s %-6s %s\n", $1, $2, $3, $4, $5}' <<<"$users"
  echo
  local def; def=$(awk -F'\t' '$4=="是"{print $2; exit}' <<<"$users" || true)
  target=${SS_RESET_USER:-}
  [[ -z $target ]] && target=$(ask "要重置密码的用户名" "$def")
  awk -F'\t' -v u="$target" '$2==u{f=1} END{exit !f}' <<<"$users" || die "用户 $target 不存在"
  pwd=${SS_RESET_PASSWORD:-}
  if [[ -z $pwd ]] && interactive; then
    pwd=$(ask_secret "新密码（至少 8 位，直接回车自动生成）")
    if [[ -n $pwd ]]; then
      pwd2=$(ask_secret "再次输入新密码")
      [[ $pwd == "$pwd2" ]] || die "两次输入的密码不一致"
    fi
  fi
  if [[ -z $pwd ]]; then pwd=$(rand_hex 6); gen=1; fi
  [[ ${#pwd} -ge 8 ]] || die "密码长度不能少于 8 位"
  LIVEMS_NEW_PASSWORD=$pwd "$bin" -c "$conf" -reset-password -user "$target" >/dev/null || die "重置失败"
  ok "已重置用户 $target 的密码（账号已启用）"
  [[ -n $gen ]] && echo "  新密码：${C_W}$pwd${C_0}   （请登录后在右上角「修改密码」中更改）"
  echo "  后台地址：http://$(lan_ip):$HTTP_PORT/"
}

# ----------------------------------------------------------------------------- 状态 / 菜单
do_status() {
  if ! installed; then echo "  状态：${C_Y}未安装${C_0}"; return; fi
  set -a; . "$ENV_FILE"; set +a
  local a z
  a=$(systemctl is-active "$SVC_APP" 2>/dev/null || true); z=$(systemctl is-active "$SVC_ZLM" 2>/dev/null || true)
  echo "  状态：已安装 ${C_W}$(cur_version)${C_0} ｜ 后台 $a ｜ 流媒体内核 $z"
  echo "  地址：http://$(lan_ip):$HTTP_PORT/ ｜ RTMP $RTMP_PORT"
}

# 标题框：按标题的实际显示宽度生成，保证左右边框对齐
banner() {
  local title="StreamSwitch 流媒体管理系统 一键部署" inner=46 w l r
  w=$(dwidth "$title")
  (( inner < w + 4 )) && inner=$(( w + 4 ))
  l=$(( (inner - w) / 2 )); r=$(( inner - w - l ))
  echo
  printf '%s  ╔%s╗\n' "$C_B$C_W" "$(repeat ═ "$inner")"
  printf '  ║%*s%s%*s║\n' "$l" '' "$title" "$r" ''
  printf '  ╚%s╝%s\n' "$(repeat ═ "$inner")" "$C_0"
}

menu() {
  while true; do
    clear 2>/dev/null || true
    banner
    do_status
    echo
    echo "   1. 安装 / 升级"
    echo "   2. 端口设置"
    echo "   3. 卸载"
    echo "   4. 重置管理员密码"
    echo "   0. 退出"
    echo
    local c; c=$(ask "请选择" "")
    case "$c" in
      1) ( do_install ) || true; pause ;;
      2) ( do_port ) || true; pause ;;
      3) ( do_uninstall ) || true; pause ;;
      4) ( do_reset_password ) || true; pause ;;
      0|q|Q) exit 0 ;;
      *) ;;
    esac
  done
}

main() {
  case "${1:-}" in
    install|upgrade) do_install ;;
    port|ports)      do_port ;;
    uninstall|remove) do_uninstall ;;
    reset-password|passwd) do_reset_password ;;
    status)          do_status ;;
    -h|--help|help)  sed -n '2,24p' "$0" | sed 's/^# \{0,1\}//' ;;
    "")              require_root; interactive || die "非交互环境请直接指定操作：install | port | uninstall | reset-password"; menu ;;
    *)               die "未知操作：$1（可用：install | port | uninstall | reset-password | status）" ;;
  esac
}

main "$@"

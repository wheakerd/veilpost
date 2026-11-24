#!/bin/sh
# This production entry point requires no Python, Node, Go SDK or Bash.
set +x
set -eu
LC_ALL=C
export LC_ALL
umask 077

fail() { printf 'ERROR: %s\n' "$*" >&2; exit 1; }

effective_uid() {
  # /proc is available on Linux even if coreutils/id has not been installed.
  while read -r field real effective rest; do
    if [ "$field" = 'Uid:' ]; then printf '%s\n' "$effective"; return; fi
  done < /proc/self/status
  return 1
}

as_root() {
  if [ "$(effective_uid)" = 0 ]; then
    "$@"
  elif command -v sudo >/dev/null 2>&1; then
    if [ -t 0 ]; then sudo -- "$@"; else sudo -n -- "$@"; fi
  else
    fail '安装缺失依赖需要 root 或可用的 sudo；请由管理员安装后再次执行。'
  fi
}

install_packages() {
  distro_id=$1
  distro_like=$2
  shift 2
  manager=''
  for distro in "$distro_id" $distro_like; do
    case "$distro" in
      debian|ubuntu) manager=apt-get ;;
      fedora|rhel|centos|rocky|almalinux|ol|amzn)
        if command -v dnf >/dev/null 2>&1; then manager=dnf; else manager=yum; fi ;;
      alpine) manager=apk ;;
      arch|manjaro) manager=pacman ;;
      opensuse*|sles|sled|suse) manager=zypper ;;
    esac
    [ -z "$manager" ] || break
  done
  [ -n "$manager" ] || fail "暂不支持此 Linux 发行版自动安装: $distro_id ($distro_like)"
  command -v "$manager" >/dev/null 2>&1 || fail "未找到发行版包管理器: $manager"
  printf '检测到 %s，使用 %s 安装缺失依赖: %s\n' "$distro_id" "$manager" "$*"
  case "$manager" in
    apt-get)
      as_root apt-get update || fail '软件源更新失败。'
      as_root env DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends "$@" || fail '依赖安装失败。' ;;
    dnf|yum) as_root "$manager" install -y "$@" || fail '依赖安装失败。' ;;
    apk) as_root apk add --no-cache "$@" || fail '依赖安装失败。' ;;
    # Do not perform a partial or full system upgrade as part of account creation.
    pacman) as_root pacman -S --needed --noconfirm "$@" || fail '依赖安装失败；请检查系统软件源与同步状态。' ;;
    zypper) as_root zypper --non-interactive install --no-recommends "$@" || fail '依赖安装失败。' ;;
  esac
}

ensure_tools() {
  [ -r /proc/sys/kernel/ostype ] || fail '此脚本需要 Linux。'
  IFS= read -r kernel < /proc/sys/kernel/ostype
  [ "$kernel" = Linux ] || fail '此脚本需要 Linux。'
  required='mkdir mktemp chmod stat'
  [ "$generate" = true ] && required="$required openssl"
  [ "$generate" = false ] && required="$required stty"
  need_coreutils=false
  need_openssl=false
  for tool in $required; do
    if ! command -v "$tool" >/dev/null 2>&1; then
      if [ "$tool" = openssl ]; then need_openssl=true; else need_coreutils=true; fi
    fi
  done
  set --
  [ "$need_coreutils" = false ] || set -- "$@" coreutils
  [ "$need_openssl" = false ] || set -- "$@" openssl
  if [ "$#" -gt 0 ]; then
    ID=''
    ID_LIKE=''
    if [ -r /etc/os-release ]; then . /etc/os-release
    elif [ -r /usr/lib/os-release ]; then . /usr/lib/os-release
    else fail '缺失依赖，且无法读取 os-release 识别 Linux 发行版。'; fi
    install_packages "$ID" "${ID_LIKE:-}" "$@"
    hash -r
  fi
  for tool in $required; do
    command -v "$tool" >/dev/null 2>&1 || fail "安装后仍找不到工具: $tool"
  done
}

restore_terminal() {
  if [ -n "${terminal_state:-}" ]; then stty "$terminal_state" || :; fi
}

run_backend() {
  if [ -n "$binary" ]; then
    "$binary" "$@"
  else
    docker compose -p "$project" -f "$root/compose.yaml" exec -T backend \
      /usr/local/bin/veilpost-api "$@"
  fi
}

main() {
  case "${1:-}" in
    -h|--help)
      printf '%s\n' 'USERNAME=admin GENERATE_PASSWORD=1 sh scripts/admin-create.sh' \
        '原生部署: VPN_API_BIN=/path/veilpost-api VPN_DATA_DIR=/path/data sh admin-create.sh' \
        'ADMIN_ACTION=reset-password 重置已有账号密码；ADMIN_ACTION=reset-two-factor 移除其两步验证。' \
        'VPN_CREDENTIALS_DIR 可指定密码文件目录；缺少 OpenSSL/coreutils 时自动安装。'
      return ;;
    '') ;;
    *) fail '不支持的位置参数；使用 --help 查看用法。' ;;
  esac
  case "$0" in */*) script_parent=${0%/*} ;; *) script_parent=. ;; esac
  script_dir=$(CDPATH= cd -- "$script_parent" && pwd -P)
  binary=${VPN_API_BIN:-}
  if [ -f "$script_dir/../compose.yaml" ]; then
    root=$(CDPATH= cd -- "$script_dir/.." && pwd -P)
  else
    root=$script_dir
    if [ -z "$binary" ] && [ -x "$root/veilpost-api" ]; then binary=$root/veilpost-api; fi
  fi
  project=${VPN_COMPOSE_PROJECT:-veilpost-dev}
  case "$project" in ''|[!a-z0-9]*|*[!a-z0-9_-]*) fail 'Invalid VPN_COMPOSE_PROJECT' ;; esac
  [ "${#project}" -le 63 ] || fail 'Invalid VPN_COMPOSE_PROJECT'
  if [ -n "$binary" ]; then
    [ -f "$binary" ] && [ -x "$binary" ] || fail 'VPN_API_BIN 必须指向可执行的后端文件。'
    [ -n "${VPN_DATA_DIR:-}" ] || fail '原生模式必须设置 VPN_DATA_DIR，指向运行中的后端数据目录。'
    credential_target=native
  else
    command -v docker >/dev/null 2>&1 || fail 'Compose 模式需要已部署的 Docker；原生部署请设置 VPN_API_BIN 和 VPN_DATA_DIR。'
    [ -f "$root/compose.yaml" ] || fail '找不到 compose.yaml。'
    credential_target=$project
  fi
  action=${ADMIN_ACTION:-create}
  case "$action" in
    create|reset-password|reset-two-factor) ;;
    *) fail 'ADMIN_ACTION 必须为 create、reset-password 或 reset-two-factor。' ;;
  esac
  case "${GENERATE_PASSWORD:-}" in
    1|true) generate=true ;;
    0|false) generate=false ;;
    '') if [ -t 0 ]; then generate=false; else generate=true; fi ;;
    *) fail 'GENERATE_PASSWORD 必须为 1、true、0 或 false。' ;;
  esac
  [ "$generate" = true ] || [ -t 0 ] || fail '非交互模式请使用 GENERATE_PASSWORD=1。'
  username=${USERNAME:-}
  if [ -z "$username" ]; then
    if [ "$generate" = true ]; then username=admin
    else printf '管理员账号: '; IFS= read -r username || fail '未输入账号。'; fi
  fi
  case "$username" in ''|[!A-Za-z0-9]*|*[!A-Za-z0-9_.-]*) fail '账号必须为 3–64 位 ASCII 字母、数字、点、下划线或连字符。' ;; esac
  [ "${#username}" -ge 3 ] && [ "${#username}" -le 64 ] || fail '账号长度必须为 3–64。'
  if [ "$action" = reset-two-factor ]; then
    # Removing the verifier needs no password; it also revokes the account's sessions.
    if run_backend admin-reset-two-factor --username "$username" >/dev/null 2>&1; then
      printf '已移除两步验证并撤销全部会话: %s\n' "$username"
      return
    fi
    fail '两步验证未重置；请检查后端状态、账号是否存在及是否已启用两步验证。'
  fi
  ensure_tools
  if [ "$generate" = true ]; then
    admin_password=$(openssl rand -hex 32) || fail '安全随机密码生成失败。'
    case "$admin_password" in ''|*[!0-9a-f]*) fail '随机密码输出格式错误。' ;; esac
    [ "${#admin_password}" -eq 64 ] || fail '随机密码长度错误。'
    directory=${VPN_CREDENTIALS_DIR:-$root/.runtime/admin-credentials}
    [ ! -L "$directory" ] || fail '密码目录不能是符号链接。'
    mkdir -p -m 700 -- "$directory"
    [ "$(stat -c %u -- "$directory")" = "$(effective_uid)" ] && \
      [ "$(stat -c %a -- "$directory")" = 700 ] || fail '密码目录必须属于当前用户且权限为 700。'
    # BusyBox requires the template to end in XXXXXX (no filename suffix).
    credentials=$(mktemp "$directory/$credential_target-$username-XXXXXX")
    chmod 600 -- "$credentials"
    # Save before invoking the backend; retain on an unknown outcome and never retry.
    printf 'username=%s\npassword=%s\n' "$username" "$admin_password" > "$credentials" || fail '密码文件写入失败；未创建账号。'
    printf '密码已保存（仅当前用户可读）: %s\n' "$credentials"
  else
    terminal_state=$(stty -g)
    trap 'restore_terminal' 0
    trap 'exit 130' INT
    trap 'exit 143' TERM HUP
    stty -echo
    printf '密码（至少 12 个字符）: '
    IFS= read -r admin_password || fail '未输入密码。'
    printf '\n再次输入密码: '
    IFS= read -r confirmation || fail '未确认密码。'
    restore_terminal
    terminal_state=''
    printf '\n'
    [ "$admin_password" = "$confirmation" ] || fail '两次密码输入不一致。'
    unset confirmation
  fi
  if printf '%s\n' "$admin_password" | run_backend "admin-$action" --username "$username" --password-stdin >/dev/null 2>&1; then
    unset admin_password
    if [ "$action" = create ]; then printf '管理员已创建: %s\n' "$username"
    else printf '密码已重置并撤销全部会话: %s\n' "$username"; fi
  else
    unset admin_password
    [ "$generate" = false ] || printf '已保留密码文件: %s\n' "$credentials" >&2
    if [ "$action" = create ]; then
      fail '账号创建未确认；请检查后端状态及账号是否已存在。命令不会重置已有密码或自动重试。'
    fi
    fail '密码重置未确认；请检查后端状态及账号是否存在。命令不会自动重试。'
  fi
}

main "$@"

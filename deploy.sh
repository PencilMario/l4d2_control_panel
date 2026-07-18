#!/usr/bin/env bash

set -euo pipefail

DEFAULT_REPO_URL="https://github.com/PencilMario/l4d2_control_panel.git"
DEFAULT_BRANCH="main"
DEFAULT_INSTALL_DIR="/opt/l4d2-control-panel"
DEFAULT_DATA_ROOT="/srv/l4d2-panel"
DEFAULT_HTTP_PORT="18081"
DEFAULT_GAME_HOST="host.docker.internal"
DEPLOY_SCRIPT_SOURCE="${BASH_SOURCE[0]:-}"

log() {
  printf '[l4d2-panel] %s\n' "$*"
}

die() {
  printf '[l4d2-panel] 错误: %s\n' "$*" >&2
  return 1
}

reset_options() {
  REPO_URL="$DEFAULT_REPO_URL"
  BRANCH="$DEFAULT_BRANCH"
  INSTALL_DIR="$DEFAULT_INSTALL_DIR"
}

usage() {
  cat <<EOF
用法: deploy.sh [选项]

  --repo URL          Git 仓库地址
  --branch NAME       部署分支
  --install-dir PATH  安装目录
  -h, --help          显示帮助
EOF
}

parse_args() {
  while (($# > 0)); do
    case "$1" in
      --repo)
        (($# >= 2)) || { die "--repo 缺少参数"; return 1; }
        REPO_URL="$2"
        shift 2
        ;;
      --branch)
        (($# >= 2)) || { die "--branch 缺少参数"; return 1; }
        BRANCH="$2"
        shift 2
        ;;
      --install-dir)
        (($# >= 2)) || { die "--install-dir 缺少参数"; return 1; }
        INSTALL_DIR="$2"
        shift 2
        ;;
      -h|--help)
        usage
        return 2
        ;;
      *)
        die "未知参数: $1"
        return 1
        ;;
    esac
  done
}

write_env_file() {
  local env_file="$1"
  local password="$2"
  local temp_file

  mkdir -p "$(dirname "$env_file")"
  temp_file="${env_file}.tmp.$$"
  umask 077
  cat > "$temp_file" <<EOF
L4D2_PANEL_ADMIN_PASSWORD=$password
L4D2_PANEL_DATA_ROOT=$DEFAULT_DATA_ROOT
L4D2_PANEL_HTTP_PORT=$DEFAULT_HTTP_PORT
L4D2_PANEL_GAME_HOST=$DEFAULT_GAME_HOST
L4D2_PANEL_DOCKER_PROXY_PORT=23750
L4D2_PANEL_DOWNLOAD_PROXY=
ALPINE_IMAGE=alpine:3.22
STEAMCMD_IMAGE=cm2network/steamcmd:root
NODE_IMAGE=node:22-alpine
GO_IMAGE=golang:1.25-alpine
EOF
  chmod 0600 "$temp_file"
  mv -f "$temp_file" "$env_file"
}

ensure_env_file() {
  local env_file="$1"
  local password="$2"
  [[ -e "$env_file" ]] || write_env_file "$env_file" "$password"
}

update_repository() {
  local repository="$1"
  local branch="$2"
  local status filtered_status="" status_line

  [[ -d "$repository/.git" ]] || { die "安装目录不是 Git 工作树: $repository"; return 1; }
  git -C "$repository" remote get-url origin >/dev/null 2>&1 || { die "安装目录缺少 origin 远端"; return 1; }

  status="$(git -C "$repository" status --porcelain --untracked-files=all)"
  while IFS= read -r status_line; do
    [[ -n "$status_line" ]] || continue
    [[ "$status_line" == "?? .env" ]] && continue
    filtered_status+="${status_line}"$'\n'
  done <<< "$status"
  status="$filtered_status"
  [[ -z "$status" ]] || { die "安装目录存在本地修改，请先提交、移走或删除这些文件"; return 1; }

  UPDATE_FROM="$(git -C "$repository" rev-parse HEAD)"
  git -C "$repository" fetch --prune origin "$branch"
  git -C "$repository" merge-base --is-ancestor HEAD "origin/$branch" || { die "本地提交无法快进到 origin/$branch"; return 1; }
  git -C "$repository" checkout -q "$branch"
  git -C "$repository" merge --ff-only "origin/$branch"
  UPDATE_TO="$(git -C "$repository" rev-parse HEAD)"
}

bootstrap_repository() {
  local install_dir="$1"
  local repository_url="$2"
  local branch="$3"

  if [[ -d "$install_dir/.git" ]]; then
    return 0
  fi
  if [[ -e "$install_dir" ]] && [[ -n "$(find "$install_dir" -mindepth 1 -maxdepth 1 -print -quit 2>/dev/null)" ]]; then
    die "安装目录已存在且不是空目录: $install_dir"
    return 1
  fi

  mkdir -p "$(dirname "$install_dir")"
  git clone --branch "$branch" --single-branch "$repository_url" "$install_dir"
}

read_env_value() {
  local env_file="$1"
  local key="$2"
  local value

  value="$(sed -n "s/^${key}=//p" "$env_file" | tail -n 1)"
  printf '%s\n' "$value"
}

wait_for_health() {
  local env_file="$1"
  local port attempts interval attempt

  port="$(read_env_value "$env_file" L4D2_PANEL_HTTP_PORT)"
  port="${port:-$DEFAULT_HTTP_PORT}"
  attempts="${HEALTH_ATTEMPTS:-60}"
  interval="${HEALTH_INTERVAL_SECONDS:-2}"

  for ((attempt = 1; attempt <= attempts; attempt++)); do
    if curl --fail --silent --show-error "http://127.0.0.1:${port}/api/health" >/dev/null; then
      return 0
    fi
    if ((attempt < attempts)); then
      sleep "$interval"
    fi
  done

  die "健康检查超时: http://127.0.0.1:${port}/api/health"
  return 1
}

deploy_stack() {
  local repository="$1"
  local env_file="$2"

  (
    cd "$repository"
    docker compose --env-file "$env_file" config --quiet
    docker compose --env-file "$env_file" --profile images build runtime-image
    docker compose --env-file "$env_file" up -d --build
    if ! wait_for_health "$env_file"; then
      docker compose --env-file "$env_file" ps >&2 || true
      docker compose --env-file "$env_file" logs --tail=100 panel socket-proxy overlay-helper >&2 || true
      return 1
    fi
  )
}

os_release_value() {
  local os_release_file="$1"
  local key="$2"
  local value

  value="$(sed -n "s/^${key}=//p" "$os_release_file" | tail -n 1)"
  value="$(printf '%s' "$value" | sed 's/^"//;s/"$//')"
  printf '%s\n' "$value"
}

install_docker_engine() {
  local distribution_id="$1"
  local codename="$2"
  local keyring_dir="${DOCKER_KEYRING_DIR:-/etc/apt/keyrings}"
  local source_file="${DOCKER_SOURCE_FILE:-/etc/apt/sources.list.d/docker.list}"
  local architecture

  architecture="$(dpkg --print-architecture)"
  apt-get update
  apt-get install -y ca-certificates curl git
  apt-get remove -y docker.io docker-compose docker-compose-v2 docker-doc podman-docker containerd runc || true
  mkdir -p "$keyring_dir" "$(dirname "$source_file")"
  curl -fsSL "https://download.docker.com/linux/${distribution_id}/gpg" -o "$keyring_dir/docker.asc"
  chmod 0644 "$keyring_dir/docker.asc"
  printf 'deb [arch=%s signed-by=%s/docker.asc] https://download.docker.com/linux/%s %s stable\n' \
    "$architecture" "$keyring_dir" "$distribution_id" "$codename" > "$source_file"
  apt-get update
  apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
}

start_docker_service() {
  if command -v systemctl >/dev/null 2>&1; then
    systemctl enable --now docker
  elif command -v service >/dev/null 2>&1; then
    service docker start
  fi
}

ensure_dependencies() {
  local os_release_file="${OS_RELEASE_FILE:-/etc/os-release}"
  local distribution_id codename

  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    if ! docker info >/dev/null 2>&1; then
      start_docker_service
      docker info >/dev/null 2>&1 || { die "Docker Engine 无法连接"; return 1; }
    fi
    command -v git >/dev/null 2>&1 || { die "缺少 git"; return 1; }
    command -v curl >/dev/null 2>&1 || { die "缺少 curl"; return 1; }
    return 0
  fi

  [[ -r "$os_release_file" ]] || { die "无法识别 Linux 发行版"; return 1; }
  distribution_id="$(os_release_value "$os_release_file" ID)"
  codename="$(os_release_value "$os_release_file" VERSION_CODENAME)"
  case "$distribution_id" in
    debian|ubuntu) ;;
    *)
      die "缺少 Docker；自动安装仅自动支持 Debian/Ubuntu"
      return 1
      ;;
  esac
  [[ -n "$codename" ]] || { die "无法读取发行版 VERSION_CODENAME"; return 1; }

  log "正在安装 Docker Engine 与 Compose 插件"
  install_docker_engine "$distribution_id" "$codename"
  start_docker_service
  command -v git >/dev/null 2>&1 || { die "git 安装失败"; return 1; }
  command -v curl >/dev/null 2>&1 || { die "curl 安装失败"; return 1; }
  docker compose version >/dev/null 2>&1 || { die "Docker Compose 插件安装失败"; return 1; }
  docker info >/dev/null 2>&1 || { die "Docker Engine 无法连接"; return 1; }
}

update_and_deploy() {
  local repository="$1"
  local branch="$2"
  local env_file="$3"

  update_repository "$repository" "$branch"
  if deploy_stack "$repository" "$env_file"; then
    return 0
  fi

  log "新版本部署失败，正在回退到提交 $UPDATE_FROM"
  git -C "$repository" reset --hard "$UPDATE_FROM"
  if ! deploy_stack "$repository" "$env_file"; then
    die "回退后服务仍未恢复，请检查 Compose 日志"
    return 1
  fi
  die "新版本部署失败，已回退并恢复旧版本"
  return 1
}

validate_host() {
  local effective_uid="$1"
  local kernel_name="$2"
  local machine_architecture="$3"

  [[ "$effective_uid" == "0" ]] || { die "请使用 root 权限运行（例如 curl ... | sudo bash）"; return 1; }
  [[ "$kernel_name" == "Linux" ]] || { die "生产部署仅支持 Linux"; return 1; }
  case "$machine_architecture" in
    x86_64|amd64) ;;
    *)
      die "生产部署仅支持 x86-64，当前架构为 $machine_architecture"
      return 1
      ;;
  esac
}

validate_current_host() {
  validate_host "$(id -u)" "$(uname -s)" "$(uname -m)"
}

generate_password() {
  if command -v openssl >/dev/null 2>&1; then
    openssl rand -hex 24
    return
  fi
  od -An -N24 -tx1 /dev/urandom | tr -d '[:space:]'
  printf '\n'
}

locate_repository() {
  local install_dir="$1"
  local repository_url="$2"
  local branch="$3"
  local script_dir=""

  if [[ -n "$DEPLOY_SCRIPT_SOURCE" ]] && [[ -f "$DEPLOY_SCRIPT_SOURCE" ]]; then
    script_dir="$(cd "$(dirname "$DEPLOY_SCRIPT_SOURCE")" && pwd)"
  fi
  if [[ -n "$script_dir" ]] && [[ -d "$script_dir/.git" ]] && [[ -f "$script_dir/docker-compose.yml" ]]; then
    printf '%s\n' "$script_dir"
    return 0
  fi

  bootstrap_repository "$install_dir" "$repository_url" "$branch"
  printf '%s\n' "$install_dir"
}

deployment_address() {
  local env_file="$1"
  local port host_address

  port="$(read_env_value "$env_file" L4D2_PANEL_HTTP_PORT)"
  port="${port:-$DEFAULT_HTTP_PORT}"
  host_address="$(hostname -I 2>/dev/null | awk '{print $1}')"
  host_address="${host_address:-127.0.0.1}"
  printf 'http://%s:%s\n' "$host_address" "$port"
}

main() {
  local repository env_file generated_password="" created_env=false address parse_status

  reset_options
  parse_args "$@" || {
    parse_status=$?
    [[ "$parse_status" == "2" ]] && return 0
    return "$parse_status"
  }
  validate_current_host
  ensure_dependencies

  repository="$(locate_repository "$INSTALL_DIR" "$REPO_URL" "$BRANCH")"
  env_file="$repository/.env"
  if [[ ! -e "$env_file" ]]; then
    generated_password="$(generate_password)"
    write_env_file "$env_file" "$generated_password"
    created_env=true
  fi

  update_and_deploy "$repository" "$BRANCH" "$env_file"
  address="$(deployment_address "$env_file")"
  log "部署完成: $address"
  log "安装目录: $repository"
  log "更新命令: sudo $repository/deploy.sh"
  if [[ "$created_env" == "true" ]]; then
    log "管理员密码: $generated_password"
    log "请立即保存该密码；后续更新不会重新生成"
  fi
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  main "$@"
fi

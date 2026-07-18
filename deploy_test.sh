#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
DEPLOY_SCRIPT="$ROOT_DIR/deploy.sh"

fail() {
  printf 'FAIL: %s\n' "$*" >&2
  exit 1
}

assert_eq() {
  local expected="$1"
  local actual="$2"
  local message="$3"
  [[ "$actual" == "$expected" ]] || fail "$message (expected '$expected', got '$actual')"
}

assert_contains() {
  local haystack="$1"
  local needle="$2"
  local message="$3"
  [[ "$haystack" == *"$needle"* ]] || fail "$message (missing '$needle')"
}

new_temp_dir() {
  mktemp -d "${TMPDIR:-/tmp}/l4d2-deploy-test.XXXXXX"
}

source_deploy() {
  [[ -f "$DEPLOY_SCRIPT" ]] || fail "deploy.sh does not exist"
  # shellcheck source=deploy.sh
  source "$DEPLOY_SCRIPT"
}

test_parse_args_defaults_and_overrides() {
  source_deploy

  reset_options
  parse_args
  assert_eq "https://github.com/PencilMario/l4d2_control_panel.git" "$REPO_URL" "default repository"
  assert_eq "main" "$BRANCH" "default branch"
  assert_eq "/opt/l4d2-control-panel" "$INSTALL_DIR" "default install directory"

  reset_options
  parse_args --repo https://example.invalid/panel.git --branch stable --install-dir /srv/panel
  assert_eq "https://example.invalid/panel.git" "$REPO_URL" "repository override"
  assert_eq "stable" "$BRANCH" "branch override"
  assert_eq "/srv/panel" "$INSTALL_DIR" "install directory override"
}

test_env_generation_and_preservation() {
  source_deploy
  local temp_dir env_file original mode
  temp_dir="$(new_temp_dir)"
  env_file="$temp_dir/.env"

  write_env_file "$env_file" "fixed-secret"
  original="$(cat "$env_file")"
  mode="$(stat -c '%a' "$env_file")"
  assert_contains "$original" "L4D2_PANEL_ADMIN_PASSWORD=fixed-secret" "generated password"
  assert_contains "$original" "L4D2_PANEL_DATA_ROOT=/srv/l4d2-panel" "default data root"
  assert_contains "$original" "L4D2_PANEL_HTTP_PORT=18081" "default HTTP port"
  assert_contains "$original" "L4D2_PANEL_GAME_HOST=host.docker.internal" "default game host"
  assert_eq "600" "$mode" "environment file permissions"

  ensure_env_file "$env_file" "replacement-secret"
  assert_eq "$original" "$(cat "$env_file")" "existing environment file must be preserved"
  rm -rf "$temp_dir"
}

configure_git_identity() {
  local repository="$1"
  git -C "$repository" config user.name "Deploy Test"
  git -C "$repository" config user.email "deploy-test@example.invalid"
}

create_git_fixture() {
  local fixture_root="$1"
  local source_repo="$fixture_root/source"
  local remote_repo="$fixture_root/remote.git"
  local install_repo="$fixture_root/install"

  git init -q --initial-branch=main "$source_repo"
  configure_git_identity "$source_repo"
  printf 'one\n' > "$source_repo/version.txt"
  git -C "$source_repo" add version.txt
  git -C "$source_repo" commit -qm initial
  git clone -q --bare "$source_repo" "$remote_repo"
  git clone -q --branch main "$remote_repo" "$install_repo"
  configure_git_identity "$install_repo"

  printf '%s\n%s\n%s\n' "$source_repo" "$remote_repo" "$install_repo"
}

test_update_rejects_dirty_tree_and_fast_forwards() {
  source_deploy
  local fixture_root source_repo remote_repo install_repo output
  fixture_root="$(new_temp_dir)"
  mapfile -t repositories < <(create_git_fixture "$fixture_root")
  source_repo="${repositories[0]}"
  remote_repo="${repositories[1]}"
  install_repo="${repositories[2]}"

  printf 'L4D2_PANEL_ADMIN_PASSWORD=preserved\n' > "$install_repo/.env"
  printf 'dirty\n' > "$install_repo/local.txt"
  if output="$(update_repository "$install_repo" main 2>&1)"; then
    fail "dirty repository update unexpectedly succeeded"
  fi
  assert_contains "$output" "本地修改" "dirty repository diagnostic"
  rm "$install_repo/local.txt"

  printf 'two\n' > "$source_repo/version.txt"
  git -C "$source_repo" add version.txt
  git -C "$source_repo" commit -qm update
  git -C "$source_repo" push -q "$remote_repo" main

  update_repository "$install_repo" main
  assert_eq "two" "$(tr -d '\r\n' < "$install_repo/version.txt")" "repository fast-forward"
  assert_contains "$(cat "$install_repo/.env")" "preserved" "environment file preserved during update"
  rm -rf "$fixture_root"
}

test_bootstrap_clones_repository() {
  source_deploy
  local fixture_root source_repo remote_repo ignored install_repo
  fixture_root="$(new_temp_dir)"
  mapfile -t repositories < <(create_git_fixture "$fixture_root")
  source_repo="${repositories[0]}"
  remote_repo="${repositories[1]}"
  ignored="${repositories[2]}"
  install_repo="$fixture_root/bootstrap-install"

  bootstrap_repository "$install_repo" "$remote_repo" main
  assert_eq "one" "$(tr -d '\r\n' < "$install_repo/version.txt")" "bootstrap clone content"
  assert_contains "$(git -C "$install_repo" remote get-url origin)" "remote.git" "bootstrap origin"
  rm -rf "$fixture_root"
}

test_compose_deploy_and_health_check() {
  source_deploy
  local temp_dir fake_bin command_log env_file
  temp_dir="$(new_temp_dir)"
  fake_bin="$temp_dir/bin"
  command_log="$temp_dir/commands.log"
  env_file="$temp_dir/.env"
  mkdir -p "$fake_bin"
  write_env_file "$env_file" fixed-secret

  cat > "$fake_bin/docker" <<'EOF'
#!/usr/bin/env bash
printf 'docker %s\n' "$*" >> "$COMMAND_LOG"
exit 0
EOF
  cat > "$fake_bin/curl" <<'EOF'
#!/usr/bin/env bash
printf 'curl %s\n' "$*" >> "$COMMAND_LOG"
exit 0
EOF
  chmod +x "$fake_bin/docker" "$fake_bin/curl"

  PATH="$fake_bin:$PATH" COMMAND_LOG="$command_log" HEALTH_ATTEMPTS=1 deploy_stack "$temp_dir" "$env_file"
  local commands
  commands="$(cat "$command_log")"
  assert_contains "$commands" "docker compose --env-file $env_file config --quiet" "compose validation"
  assert_contains "$commands" "docker compose --env-file $env_file --profile images build runtime-image" "runtime image build"
  assert_contains "$commands" "docker compose --env-file $env_file up -d --build" "compose service start"
  assert_contains "$commands" "curl --fail --silent --show-error http://127.0.0.1:18081/api/health" "health endpoint"
  rm -rf "$temp_dir"
}

test_docker_install_on_debian_and_reject_unknown_distribution() {
  source_deploy
  local temp_dir fake_bin command_log marker os_release output
  temp_dir="$(new_temp_dir)"
  fake_bin="$temp_dir/bin"
  command_log="$temp_dir/commands.log"
  marker="$temp_dir/docker-installed"
  os_release="$temp_dir/os-release"
  mkdir -p "$fake_bin"
  cat > "$os_release" <<'EOF'
ID=debian
VERSION_CODENAME=bookworm
EOF

  cat > "$fake_bin/git" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
  cat > "$fake_bin/curl" <<'EOF'
#!/usr/bin/env bash
printf 'curl %s\n' "$*" >> "$COMMAND_LOG"
output=''
while (($#)); do
  if [[ "$1" == "-o" ]]; then output="$2"; shift 2; else shift; fi
done
if [[ -n "$output" ]]; then mkdir -p "$(dirname "$output")"; : > "$output"; fi
EOF
  cat > "$fake_bin/apt-get" <<'EOF'
#!/usr/bin/env bash
printf 'apt-get %s\n' "$*" >> "$COMMAND_LOG"
if [[ "$*" == *"docker-ce"* ]]; then : > "$DOCKER_MARKER"; fi
EOF
  cat > "$fake_bin/docker" <<'EOF'
#!/usr/bin/env bash
[[ -f "$DOCKER_MARKER" ]] || exit 1
exit 0
EOF
  cat > "$fake_bin/dpkg" <<'EOF'
#!/usr/bin/env bash
printf 'amd64\n'
EOF
  cat > "$fake_bin/systemctl" <<'EOF'
#!/usr/bin/env bash
printf 'systemctl %s\n' "$*" >> "$COMMAND_LOG"
exit 0
EOF
  chmod +x "$fake_bin"/*

  PATH="$fake_bin:/usr/bin:/bin" COMMAND_LOG="$command_log" DOCKER_MARKER="$marker" OS_RELEASE_FILE="$os_release" DOCKER_KEYRING_DIR="$temp_dir/keyrings" DOCKER_SOURCE_FILE="$temp_dir/docker.list" ensure_dependencies
  local commands
  commands="$(cat "$command_log")"
  assert_contains "$commands" "apt-get remove -y docker.io docker-compose docker-compose-v2 docker-doc podman-docker containerd runc" "conflicting Docker packages"
  assert_contains "$commands" "apt-get install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin" "Docker packages"

  printf 'ID=arch\n' > "$os_release"
  rm -f "$marker"
  if output="$(PATH="$fake_bin:/usr/bin:/bin" COMMAND_LOG="$command_log" DOCKER_MARKER="$marker" OS_RELEASE_FILE="$os_release" DOCKER_KEYRING_DIR="$temp_dir/keyrings" DOCKER_SOURCE_FILE="$temp_dir/docker.list" ensure_dependencies 2>&1)"; then
    fail "unsupported distribution unexpectedly installed Docker"
  fi
  assert_contains "$output" "仅自动支持 Debian/Ubuntu" "unsupported distribution diagnostic"
  rm -rf "$temp_dir"
}

test_failed_update_rolls_back_repository() {
  source_deploy
  local fixture_root source_repo remote_repo install_repo old_commit output
  fixture_root="$(new_temp_dir)"
  mapfile -t repositories < <(create_git_fixture "$fixture_root")
  source_repo="${repositories[0]}"
  remote_repo="${repositories[1]}"
  install_repo="${repositories[2]}"
  old_commit="$(git -C "$install_repo" rev-parse HEAD)"

  printf 'two\n' > "$source_repo/version.txt"
  git -C "$source_repo" add version.txt
  git -C "$source_repo" commit -qm update
  git -C "$source_repo" push -q "$remote_repo" main

  DEPLOY_STACK_CALLS=0
  deploy_stack() {
    DEPLOY_STACK_CALLS=$((DEPLOY_STACK_CALLS + 1))
    ((DEPLOY_STACK_CALLS > 1))
  }

  if output="$(update_and_deploy "$install_repo" main "$install_repo/.env" 2>&1)"; then
    fail "failed deployment unexpectedly succeeded"
  fi
  assert_contains "$output" "回退" "rollback diagnostic"
  assert_eq "$old_commit" "$(git -C "$install_repo" rev-parse HEAD)" "repository rollback commit"
  assert_eq "one" "$(tr -d '\r\n' < "$install_repo/version.txt")" "repository rollback content"
  rm -rf "$fixture_root"
}

test_host_validation_and_password_generation() {
  source_deploy
  local output password

  validate_host 0 Linux x86_64
  if output="$(validate_host 1000 Linux x86_64 2>&1)"; then
    fail "non-root host unexpectedly passed validation"
  fi
  assert_contains "$output" "root" "root validation diagnostic"
  if output="$(validate_host 0 Darwin x86_64 2>&1)"; then
    fail "non-Linux host unexpectedly passed validation"
  fi
  assert_contains "$output" "Linux" "Linux validation diagnostic"
  if output="$(validate_host 0 Linux aarch64 2>&1)"; then
    fail "non-x86-64 host unexpectedly passed validation"
  fi
  assert_contains "$output" "x86-64" "architecture validation diagnostic"

  password="$(generate_password)"
  ((${#password} >= 32)) || fail "generated password is too short"
  [[ "$password" =~ ^[A-Za-z0-9_-]+$ ]] || fail "generated password contains unsafe characters"
}

test_main_orchestrates_local_deployment() {
  source_deploy
  local temp_dir command_log output
  temp_dir="$(new_temp_dir)"
  command_log="$temp_dir/commands.log"
  mkdir -p "$temp_dir/.git"
  : > "$temp_dir/docker-compose.yml"

  validate_current_host() { printf 'host\n' >> "$command_log"; }
  ensure_dependencies() { printf 'dependencies\n' >> "$command_log"; }
  locate_repository() { printf '%s\n' "$temp_dir"; }
  generate_password() { printf 'fixed-main-secret\n'; }
  update_and_deploy() { printf 'deploy %s %s %s\n' "$1" "$2" "$3" >> "$command_log"; }
  deployment_address() { printf 'http://192.0.2.10:18081\n'; }

  output="$(main --branch stable 2>&1)"
  assert_contains "$(cat "$command_log")" "host" "host validation call"
  assert_contains "$(cat "$command_log")" "dependencies" "dependency check call"
  assert_contains "$(cat "$command_log")" "deploy $temp_dir stable $temp_dir/.env" "deployment call"
  assert_contains "$(cat "$temp_dir/.env")" "L4D2_PANEL_ADMIN_PASSWORD=fixed-main-secret" "main generated environment"
  assert_contains "$output" "http://192.0.2.10:18081" "deployment address summary"
  assert_contains "$output" "fixed-main-secret" "first deployment password summary"
  rm -rf "$temp_dir"
}

main() {
  test_parse_args_defaults_and_overrides
  test_env_generation_and_preservation
  test_update_rejects_dirty_tree_and_fast_forwards
  test_bootstrap_clones_repository
  test_compose_deploy_and_health_check
  test_docker_install_on_debian_and_reject_unknown_distribution
  test_failed_update_rolls_back_repository
  test_host_validation_and_password_generation
  test_main_orchestrates_local_deployment
  printf 'PASS: deploy script behavior tests\n'
}

main "$@"

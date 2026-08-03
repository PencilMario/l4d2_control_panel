# GitHub Release 下载加速系统设置 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use aegis:subagent-driven-development (recommended) or aegis:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 GitHub Release 下载加速地址从环境变量迁移到系统设置，默认空值，并支持保存后立即生效。

**Architecture:** 使用现有 SQLite `system_settings` 表持久化地址。HTTP API 提供 GET/PUT，React 系统设置页提供输入与保存；Release Client 每次下载前通过设置读取器获取当前值。GitHub API 查询保持直连，启用加速时不向第三方转发 GitHub token。

**Tech Stack:** Go, SQLite, chi, React/TypeScript, Vitest.

**Baseline / Authority Refs:** `internal/store/job_history.go` 的普通系统设置模式、`internal/httpapi/server.go` 的设置路由、`web/src/app/App.tsx` 的系统设置页、`internal/releases/github.go` 的 URL 重写逻辑。

**Compatibility Boundary:** 保持现有 Release API、GitHub API 直连、token 隔离和下载行为；环境变量不再作为运行时后备。默认值为空，空值表示直连。部署时两台现有服务器显式保存当前加速地址。

**Verification:** Go 单元/API/存储测试，前端设置页测试，`go test ./... -count=1`，`npm test -- --run`，`git diff --check`，提交后检查两台服务器健康状态与数据库设置生效。

---

### Task 1: Persist and expose accelerator setting

**Files:**
- Modify: `internal/store/job_history.go`
- Test: `internal/store/store_test.go`
- Modify: `internal/httpapi/server.go`
- Test: `internal/httpapi/server_test.go`

**Why this task exists:** 设置必须跨重启保存，默认空值，并通过现有认证设置 API 读取和更新。

**Impact / Compatibility:** 复用 `system_settings`，不新增迁移；新增 `/api/settings/github-releases` GET/PUT，不影响现有设置接口。只接受空值或 HTTPS、无 query/fragment 的地址。

**Verification:** 先运行新增存储/API 测试确认缺少实现而失败，再实现并运行相关 Go 测试。

### Task 2: Make Release downloads read current setting

**Files:**
- Modify: `internal/releases/github.go`
- Test: `internal/releases/github_test.go`
- Modify: `cmd/panel/main.go`

**Why this task exists:** 管理员保存后无需重启，下一次 Release 下载即可使用新地址。

**Impact / Compatibility:** 保留现有 `Client` 行为测试；新增可选设置读取接口，未配置时为空。移除环境变量作为 Client 的配置来源，并让 API、自动化和同步器共享同一个动态 Client。

**Verification:** 新增测试覆盖运行时读取变更、空值直连、加速下载不携带 token；运行 `go test ./internal/releases ./internal/httpapi ./internal/store`。

### Task 3: Add system settings UI

**Files:**
- Modify: `web/src/app/App.tsx`
- Test: `web/src/app/App.test.tsx`

**Why this task exists:** 管理员能在系统设置中查看、修改、清空加速地址，并获得保存状态与错误反馈。

**Impact / Compatibility:** 沿用现有设置卡片、锁机制和 API 客户端；初始值为空，不把地址硬编码为表单默认值。

**Verification:** 先运行新增前端测试确认控件不存在而失败，再验证加载、保存、清空和非法地址提示。

### Task 4: Retire environment configuration and deploy

**Files:**
- Modify: `internal/config/config.go`, tests, `docker-compose.yml`, `.env.example`, `deploy.sh`, `README.md`, related tests

**Why this task exists:** 避免环境变量与系统设置形成双重配置；现有两台服务器通过系统设置显式保留当前加速地址。

**Impact / Compatibility:** 移除环境变量说明和注入；服务器数据库设置写入 `https://releases.0721play.top/`，应用默认仍为空。

**Verification:** 全量 Go/前端测试、diff 检查、提交推送；两台服务器重建后健康检查并确认设置 API 返回目标地址。

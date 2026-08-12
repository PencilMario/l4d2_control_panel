# Evidence

## Remote deployment

- 目标：`steam@100.73.249.118`，服务地址 `http://100.73.249.118:18081/`。
- 重建命令：`docker compose -p l4d2-control-panel up -d --no-deps --force-recreate panel`。
- 运行镜像：`sha256:e5a919a24e274326b1f350bca9877995993f34e91afc808c505bd6e85d8b4262`。
- `docker inspect`：Panel health 为 `healthy`。
- `curl -fsS http://127.0.0.1:18081/api/health`：返回 `{"containers_running":6,"database":"ok","docker_version":"29.2.1","status":"ok"}`。

## Breakpad tools and symbol indexing

- 容器文件检查确认 `/usr/local/bin/dump_syms` 和 `/usr/local/bin/minidump_stackwalk` 存在。
- `dump_syms --help` 输出 `Usage: dump_syms [OPTION] <binary-with-debugging-info> [directories-for-debug-file]`。
- Panel 启动日志：`scanned=105543 candidates=396 generated=248 skipped=0 duplicates=142 failed=6`。
- 失败项均为 `libcef.so` 或 `steamclient.so`，错误为 `Breakpad symbol output exceeds the size limit`；这属于已声明的单文件符号大小限制，不影响其余模块生成。
- 本次收尾未重新触发游戏实例安装；Panel 启动扫描已实际读取 release 与实例 overlay 路径，并将结果写入 `/home/steam/l4d2-panel-data/panel/crash-dumps`。
- 运行时文件检查确认 `engine_srv.so`、`server_srv.so`、`srcds_linux`、`sourcemod.2.l4d2.so`、`metamod.2.l4d2.so` 的 Breakpad lookup 文件均存在；目录统计为 250 个 manifest、500 个 `.sym` 文件（含内置/历史重复对象）。

## Browser verification

- 访问带缓存参数的首页后，`index-DGEHwafY.js` 与 `index-BAhLgeZK.css` 均返回 200，页面渲染出控制面板，控制台错误为 0。
- 进入“崩溃报告”页后，`GET /api/crash-reports`、报告详情和 `download?file=stackwalk` 均返回 200；页面显示已有 1 条报告，控制台错误为 0。
- 首次页面加载出现旧资源指纹 404，是 Playwright 浏览器缓存复用了旧 HTML/资源组合；带查询参数重新加载后资源指纹一致，未修改产品代码。
- 重新建立浏览器目标页后，390x844 视口实际生效：`innerWidth=390`、`scrollWidth=375`，首页与“崩溃报告”页面均无控制台错误。
- 移动端“崩溃报告”页面确认显示报告列表与详情；`GET /api/crash-reports`、详情与 stackwalk 下载请求均为 200。

## Worktree and scope

- 符号生成 worktree `feature/symbol-generation` 当前基于 `5fe9de4`，仅有部署临时目录待清理。
- 主 worktree 的 `deployment_test.go`、`internal/config/config.go` 用户未提交修改未被覆盖、提交或回退。
- 未修改远端 `.env`、Panel 数据目录或其他服务容器。

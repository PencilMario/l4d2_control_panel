import { copyFile, mkdir } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import { join } from "node:path";
const goroot = execFileSync("go", ["env", "GOROOT"], { encoding: "utf8" }).trim();
await mkdir("public", { recursive: true });
execFileSync("go", ["build", "-o", "web/public/vpk-cleaner.wasm", "./cmd/vpk-cleaner-wasm"], { cwd: "..", stdio: "inherit", env: { ...process.env, GOOS: "js", GOARCH: "wasm" } });
try { await copyFile(join(goroot, "lib", "wasm", "wasm_exec.js"), "public/wasm_exec.js"); }
catch { await copyFile(join(goroot, "misc", "wasm", "wasm_exec.js"), "public/wasm_exec.js"); }

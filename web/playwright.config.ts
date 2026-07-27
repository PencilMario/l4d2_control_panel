import { defineConfig, devices } from "@playwright/test";

const e2ePort = process.env.L4D2_E2E_PORT ?? "18082";
const e2eBaseURL = `http://127.0.0.1:${e2ePort}`;
const existingNoProxy = process.env.NO_PROXY ?? process.env.no_proxy ?? "";
const noProxy = ["127.0.0.1", "localhost", existingNoProxy]
  .filter(Boolean)
  .join(",");
process.env.NO_PROXY = noProxy;
process.env.no_proxy = noProxy;

export default defineConfig({
  testDir: "./e2e",
  timeout: 90_000,
  expect: { timeout: 10_000 },
  fullyParallel: false,
  workers: 1,
  reporter: "list",
  use: {
    baseURL: e2eBaseURL,
    trace: "retain-on-failure",
    screenshot: "only-on-failure",
  },
  webServer: {
    command: "npm run e2e:server",
    env: {
      ...process.env,
      L4D2_E2E_LISTEN: `127.0.0.1:${e2ePort}`,
    },
    url: `${e2eBaseURL}/api/health`,
    timeout: 180_000,
    reuseExistingServer: false,
    stdout: "pipe",
    stderr: "pipe",
  },
  projects: [
    {
      name: "desktop",
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 1463, height: 800 },
      },
    },
    {
      name: "mobile",
      use: {
        ...devices["Desktop Chrome"],
        viewport: { width: 390, height: 844 },
        isMobile: true,
      },
    },
  ],
});

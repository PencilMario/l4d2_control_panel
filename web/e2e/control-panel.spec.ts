import { expect, test, type Page } from "@playwright/test";

const packageZip = Buffer.from(
  "UEsDBBQAAAAIAIQg71zL9TlxGQAAABEAAAAOAAAAY2ZnL3BsdWdpbi5jZmcqzo1PLkssUkjLrCgpLUpVMAQAAAD//wMAUEsBAhQAFAAAAAgAhCDvXMv1OXEZAAAAEQAAAA4AAAAAAAAAAAAAAAAAAAAAAGNmZy9wbHVnaW4uY2ZnUEsFBgAAAAABAAEAPAAAAEUAAAAAAA==",
  "base64",
);
const privateReplacementZip = Buffer.from(
  "UEsDBBQAAAAIAEgX8Fz3sBxbHQAAABUAAAAQAAAAaW1wb3J0ZWQvbmV3LmNmZ8rMLcgvKklNUShKLchJTE7NTc0r4QIAAAD//wMAUEsBAhQAFAAAAAgASBfwXPewHFsdAAAAFQAAABAAAAAAAAAAAAAAAAAAAAAAAGltcG9ydGVkL25ldy5jZmdQSwUGAAAAAAEAAQA+AAAASwAAAAAA",
  "base64",
);

type FixtureJob = {
  ID: string;
  InstanceID: string;
  Type: string;
  Status: string;
};

async function captureJob(
  page: Page,
  path: string,
  action: () => Promise<void>,
  method = "POST",
): Promise<FixtureJob> {
  const response = page.waitForResponse((candidate) => {
    const request = candidate.request();
    return (
      request.method() === method &&
      new URL(candidate.url()).pathname.includes(path) &&
      candidate.status() === 202
    );
  });
  await action();
  return (await response).json();
}

async function jobStatus(page: Page, id: string): Promise<string> {
  return page.evaluate(async (jobID) => {
    const response = await fetch(`/api/jobs/${jobID}`);
    if (!response.ok) {
      throw new Error(`job request failed with HTTP ${response.status}`);
    }
    return ((await response.json()) as FixtureJob).Status;
  }, id);
}

async function waitForJob(page: Page, id: string) {
  await expect.poll(() => jobStatus(page, id)).toBe("succeeded");
}

async function answerDialogs(page: Page, answers: Array<string | boolean>) {
  let index = 0;
  const handler = async (dialog: import("@playwright/test").Dialog) => {
    const answer = answers[index++];
    if (typeof answer === "string") await dialog.accept(answer);
    else if (answer) await dialog.accept();
    else await dialog.dismiss();
    if (index === answers.length) page.off("dialog", handler);
  };
  page.on("dialog", handler);
}

async function consolePosition(page: Page) {
  return page.locator(".terminal-modal pre").evaluate((output) => ({
    top: output.scrollTop,
    bottom: output.scrollHeight - output.clientHeight,
    clientWidth: output.clientWidth,
    scrollWidth: output.scrollWidth,
  }));
}

async function privateTree(page: Page, mobile: boolean) {
  if (mobile) {
    const drawer = page.getByRole("dialog", { name: "私有文件目录" });
    if (!(await drawer.isVisible())) {
      await page.getByRole("button", { name: "打开文件树" }).click();
    }
    return drawer.getByRole("tree", { name: "私有文件树" });
  }
  return page.getByRole("tree", { name: "私有文件树" });
}

async function closePrivateTree(page: Page, mobile: boolean) {
  if (mobile) await page.getByRole("button", { name: "关闭文件树" }).click();
}

test("real HTTP administration journey survives refresh and streams recovery state", async ({
  page,
}, testInfo) => {
  const mobile = testInfo.project.name === "mobile";
  const suffix = mobile ? "Mobile" : "Desktop";
  const instanceName = `E2E ${suffix}`;
  const secondInstanceName = `E2E ${suffix} Alt`;
  const port = mobile ? 27115 : 27015;
  const secondPort = port + 20;
  const packageAName = `fixture-${suffix.toLowerCase()}-a.zip`;
  const packageBName = `fixture-${suffix.toLowerCase()}-b.zip`;
  const initialExtraArgs = `-strictportbind +hostname "E2E ${suffix}"`;
  const secondExtraArgs = `+hostname "E2E ${suffix} Alt"`;
  const changedExtraArgs = `+hostname "E2E ${suffix} Changed"`;
  const instanceCard = (name: string) =>
    page.locator("article.card").filter({
      has: page.getByRole("heading", { name, exact: true }),
    });
  let vpkChunks = 0;
  page.on("request", (request) => {
    if (
      request.method() === "PATCH" &&
      request.url().includes("/api/content/vpk/uploads/")
    ) {
      vpkChunks += 1;
    }
  });

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "管理员认证" })).toBeVisible();
  await page.getByLabel("管理员密码").fill("correct horse battery staple");
  await page.getByRole("button", { name: "进入作战室" }).click();
  await expect(page.getByRole("heading", { name: "服务器作战室" })).toBeVisible();

  await page.evaluate(async () => {
    const response = await fetch("/api/settings/jobs", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ successful_job_limit: 40 }),
    });
    if (!response.ok) {
      throw new Error(`reset job settings failed with HTTP ${response.status}`);
    }
  });

  await page.getByRole("button", { name: "内容仓库" }).click();
  for (const name of [packageAName, packageBName]) {
    await page.locator('input[accept=".zip"]').setInputFiles({
      name,
      mimeType: "application/zip",
      buffer: packageZip,
    });
    await expect(page.getByText(new RegExp(name.replace(".", "\\.")))).toBeVisible();
  }

  await page.getByRole("button", { name: "总览" }).click();
  await page.getByRole("button", { name: "创建实例" }).click();
  await page.getByLabel("名称").fill(instanceName);
  await page.getByRole("combobox", { name: "模式" }).selectOption("coop");
  await page.getByLabel("最大玩家").fill("8");
  await page.getByLabel("游戏端口").fill(String(port));
  await page.getByLabel("SourceTV 端口").fill(String(port + 5));
  await page.getByLabel("插件端口").fill(`${port + 6}, ${port + 7}`);
  await page.getByLabel("模式").selectOption("coop");
  await page.getByLabel("最大玩家").fill("8");
  await page.getByLabel("插件包").selectOption({
    label: `${packageAName} · ${packageAName}`,
  });
  await page.getByLabel("额外 SRCDS 启动项").fill(initialExtraArgs);
  const preview = page.getByLabel("启动指令预览");
  await expect(preview).toContainText(
    `./srcds_run -game left4dead2 -console -port ${port} -tickrate 100 +map c2m1_highway +mp_gamemode coop -maxplayers 8 +tv_enable 1 +tv_port ${port + 5} ${initialExtraArgs}`,
  );
  const modalLayout = await page.locator(".instance-config-modal").evaluate((modal) => {
    const box = modal.getBoundingClientRect();
    const fields = modal
      .querySelector(".instance-config-fields")!
      .getBoundingClientRect();
    const command = modal
      .querySelector(".command-section")!
      .getBoundingClientRect();
    const controlsFit = Array.from(
      modal.querySelectorAll("input, select, textarea"),
    ).every(
      (control) => control.scrollWidth <= control.clientWidth,
    );
    return {
      left: box.left,
      top: box.top,
      right: box.right,
      bottom: box.bottom,
      scrollWidth: modal.scrollWidth,
      clientWidth: modal.clientWidth,
      fieldsBottom: fields.bottom,
      fieldsLeft: fields.left,
      fieldsRight: fields.right,
      commandTop: command.top,
      commandLeft: command.left,
      commandRight: command.right,
      controlsFit,
    };
  });
  expect.soft(modalLayout.left).toBeGreaterThanOrEqual(0);
  expect.soft(modalLayout.top).toBeGreaterThanOrEqual(0);
  expect.soft(modalLayout.right).toBeLessThanOrEqual(
    page.viewportSize()!.width,
  );
  expect.soft(modalLayout.bottom).toBeLessThanOrEqual(
    page.viewportSize()!.height,
  );
  expect.soft(modalLayout.scrollWidth).toBeLessThanOrEqual(
    modalLayout.clientWidth,
  );
  expect.soft(modalLayout.commandTop).toBeGreaterThanOrEqual(
    modalLayout.fieldsBottom,
  );
  expect
    .soft(Math.abs(modalLayout.commandLeft - modalLayout.fieldsLeft))
    .toBeLessThanOrEqual(1);
  expect
    .soft(Math.abs(modalLayout.commandRight - modalLayout.fieldsRight))
    .toBeLessThanOrEqual(1);
  expect.soft(modalLayout.controlsFit).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath(`${testInfo.project.name}-instance-config.png`),
    fullPage: true,
  });
  await page.getByRole("button", { name: "创建", exact: true }).click();

  let card = instanceCard(instanceName);
  await expect(card).toContainText("未安装");
  await expect(card).toContainText(packageAName);
  await expect(card).toContainText("待应用");

  await card.getByRole("button", { name: "更新" }).click();
  await expect(page.getByRole("dialog", { name: "重新安装实例组件" })).toBeVisible();
  await expect(page.getByRole("checkbox", { name: "重新安装游戏本体" })).toBeChecked();
  await expect(page.getByRole("checkbox", { name: "重新安装实例插件包" })).toBeChecked();
  const reinstallRequest = page.waitForRequest((request) =>
    request.method() === "POST" && new URL(request.url()).pathname.endsWith("/game-update"),
  );
  const reinstallJob = await captureJob(page, "/game-update", () =>
    page.getByRole("button", { name: "确认重新安装" }).click(),
  );
  expect(await (await reinstallRequest).postDataJSON()).toEqual({
    confirm: true,
    reinstall_game: true,
    reinstall_package: true,
  });
  await waitForJob(page, reinstallJob.ID);

  await page.getByRole("button", { name: "创建实例" }).click();
  await page.getByLabel("名称").fill(secondInstanceName);
  await page.getByRole("combobox", { name: "模式" }).selectOption("coop");
  await page.getByLabel("最大玩家").fill("8");
  await page.getByLabel("游戏端口").fill(String(secondPort));
  await page.getByLabel("SourceTV 端口").fill(String(secondPort + 5));
  await page.getByLabel("插件端口").fill(String(secondPort + 6));
  await page.getByLabel("插件包").selectOption({
    label: `${packageBName} · ${packageBName}`,
  });
  await page.getByLabel("额外 SRCDS 启动项").fill(secondExtraArgs);
  await expect(page.getByLabel("启动指令预览")).toContainText(
    `-port ${secondPort}`,
  );
  await expect(page.getByLabel("启动指令预览")).toContainText(
    secondExtraArgs,
  );
  await page.getByRole("button", { name: "创建", exact: true }).click();

  let secondCard = instanceCard(secondInstanceName);
  await expect(secondCard).toContainText(packageBName);
  await expect(secondCard).toContainText("待应用");
  const secondStartJob = await captureJob(page, "/actions", () =>
    secondCard.getByRole("button", { name: "启动" }).click(),
  );
  await waitForJob(page, secondStartJob.ID);
  await page.reload();
  secondCard = instanceCard(secondInstanceName);
  await expect(secondCard).toContainText("运行中");
  await expect(secondCard).toContainText(packageBName);
  await expect(secondCard).not.toContainText("待应用");

  card = instanceCard(instanceName);
  const startJob = await captureJob(page, "/actions", () =>
    card.getByRole("button", { name: "启动" }).click(),
  );
  await expect.poll(() => jobStatus(page, startJob.ID)).toBe("running");
  await page.reload();
  await expect(page.getByRole("heading", { name: "服务器作战室" })).toBeVisible();
  await waitForJob(page, startJob.ID);
  await page.reload();
  card = instanceCard(instanceName);
  await expect(card).toContainText("运行中");
  await expect(card).toContainText(packageAName);
  await expect(card).not.toContainText("待应用");
  await expect(card).toContainText("当前地图");
  await expect(card).toContainText("c2m1_highway");
  await expect(card.locator(".player-capacity")).toContainText("1 / 8");
  await expect(card.getByRole("button", { name: `停止 ${instanceName}` })).toBeVisible();
  await expect(card.getByRole("button", { name: "控制台" })).toBeVisible();
  await expect(card.getByRole("button", { name: "玩家" })).toBeVisible();
  await expect(card.getByRole("button", { name: `配置 ${instanceName}` })).toBeVisible();

  const performance = card.locator(".performance-panel");
  await expect(performance).toContainText("镜像大小");
  await expect(performance).toContainText("3 GiB");
  await expect(performance.getByText("玩家", { exact: true })).toHaveCount(0);
  await expect(performance).toContainText("12.5%");
  await expect(performance).toContainText("768 MiB / 2 GiB (37.5%)");
  await expect(performance).toContainText("128 B/s");
  await expect(performance).toContainText("累计 4 KiB");
  await expect(performance).toContainText("64 B/s");
  await expect(performance).toContainText("累计 2 KiB");
  await expect(performance).toContainText("32 B/s");
  await expect(performance).toContainText("累计 1 KiB");
  await expect(performance).toContainText("16 B/s");
  await expect(performance).toContainText("累计 512 B");
  await expect(performance).toContainText("24");
  await expect(performance).toContainText("1h 0s");
  await expect(performance).toContainText("2.5 ms");

  const chart = performance.getByTestId("performance-chart");
  const chartModes = performance.getByRole("group", { name: "性能图表指标" });
  await expect(chart).toHaveAttribute("data-point-count", "2");
  await expect(chart).toHaveAttribute("data-series-count", "1");
  const historyBoundary = await page.evaluate(async (name) => {
    const instances = await (await fetch("/api/instances")).json();
    const item = instances.find((candidate: any) => (candidate.name ?? candidate.Name) === name);
    const id = item.id ?? item.ID;
    const response = await fetch(`/api/instances/${id}/performance-history`);
    if (!response.ok) throw new Error(`history request failed with HTTP ${response.status}`);
    const points = await response.json();
    return {
      firstNetworkRX: points[0].network_rx_bytes_per_sec,
      firstDiskRead: points[0].block_read_bytes_per_sec,
      secondNetworkRX: points[1].network_rx_bytes_per_sec,
      secondNetworkTX: points[1].network_tx_bytes_per_sec,
      secondDiskRead: points[1].block_read_bytes_per_sec,
      secondDiskWrite: points[1].block_write_bytes_per_sec,
    };
  }, instanceName);
  expect(historyBoundary).toEqual({
    firstNetworkRX: null,
    firstDiskRead: null,
    secondNetworkRX: 0,
    secondNetworkTX: 0,
    secondDiskRead: 0,
    secondDiskWrite: 0,
  });
  await expect(chartModes.getByRole("button", { name: "CPU" })).toHaveAttribute("aria-pressed", "true");
  for (const mode of ["内存", "网络", "磁盘", "CPU"]) {
    await chartModes.getByRole("button", { name: mode }).click();
    await expect(chartModes.getByRole("button", { name: mode })).toHaveAttribute("aria-pressed", "true");
  }
  await chartModes.getByRole("button", { name: "网络" }).click();
  await expect(performance.locator(".performance-legend")).toContainText("下载");
  await expect(performance.locator(".performance-legend")).toContainText("上传");
  await expect(performance.locator(".performance-legend")).not.toContainText("网络 RX");
  await expect(performance.locator(".performance-legend")).not.toContainText("网络 TX");
  await expect(chart).toHaveAttribute("data-series-count", "2");
  await chartModes.getByRole("button", { name: "磁盘" }).click();
  await expect(performance.locator(".performance-legend")).toContainText("磁盘读");
  await expect(performance.locator(".performance-legend")).toContainText("磁盘写");
  await expect(chart).toHaveAttribute("data-series-count", "2");

  const cardLayout = await card.evaluate((element) => {
    const bounds = (node: Element) => {
      const box = node.getBoundingClientRect();
      return { left: box.left, top: box.top, right: box.right, bottom: box.bottom };
    };
    const cardBox = element.getBoundingClientRect();
    const performanceBox = element.querySelector(".performance-panel")!.getBoundingClientRect();
    const chartBox = element.querySelector(".performance-chart")!.getBoundingClientRect();
    const modesBox = element.querySelector(".performance-modes")!.getBoundingClientRect();
    const actionsBox = element.querySelector(".actions")!.getBoundingClientRect();
    return {
      cardLeft: cardBox.left,
      cardRight: cardBox.right,
      performanceLeft: performanceBox.left,
      performanceRight: performanceBox.right,
      chartLeft: chartBox.left,
      chartRight: chartBox.right,
      chartHeight: chartBox.height,
      modesLeft: modesBox.left,
      modesRight: modesBox.right,
      actionsLeft: actionsBox.left,
      actionsRight: actionsBox.right,
      modeButtons: Array.from(element.querySelectorAll(".performance-modes button")).map(
        (button) => ({ ...bounds(button), textFits: button.scrollWidth <= button.clientWidth }),
      ),
      actionButtons: Array.from(element.querySelectorAll(".actions button")).map(
        (button) => ({ ...bounds(button), textFits: button.scrollWidth <= button.clientWidth }),
      ),
    };
  });
  const overviewOverflow = await page.evaluate(() => ({
    documentScrollWidth: document.documentElement.scrollWidth,
    documentClientWidth: document.documentElement.clientWidth,
    bodyScrollWidth: document.body.scrollWidth,
    bodyClientWidth: document.body.clientWidth,
  }));
  const viewport = page.viewportSize()!;
  const assertInsideCardAndViewport = (box: {
    left: number;
    top: number;
    right: number;
    bottom: number;
    textFits: boolean;
  }) => {
    expect.soft(box.left).toBeGreaterThanOrEqual(cardLayout.cardLeft - 1);
    expect.soft(box.right).toBeLessThanOrEqual(cardLayout.cardRight + 1);
    expect.soft(box.left).toBeGreaterThanOrEqual(-1);
    expect.soft(box.right).toBeLessThanOrEqual(viewport.width + 1);
    expect.soft(box.top).toBeGreaterThanOrEqual(-1);
    expect.soft(box.bottom).toBeLessThanOrEqual(viewport.height + 1);
    expect.soft(box.textFits).toBe(true);
  };
  expect.soft(cardLayout.cardLeft).toBeGreaterThanOrEqual(0);
  expect.soft(cardLayout.cardRight).toBeLessThanOrEqual(viewport.width);
  expect.soft(cardLayout.performanceLeft).toBeGreaterThanOrEqual(cardLayout.cardLeft);
  expect.soft(cardLayout.performanceRight).toBeLessThanOrEqual(cardLayout.cardRight);
  expect.soft(cardLayout.chartLeft).toBeGreaterThanOrEqual(cardLayout.cardLeft);
  expect.soft(cardLayout.chartRight).toBeLessThanOrEqual(cardLayout.cardRight);
  expect.soft(cardLayout.chartHeight).toBe(mobile ? 190 : 210);
  expect.soft(cardLayout.modesLeft).toBeGreaterThanOrEqual(cardLayout.cardLeft);
  expect.soft(cardLayout.modesRight).toBeLessThanOrEqual(cardLayout.cardRight);
  expect.soft(cardLayout.actionsLeft).toBeGreaterThanOrEqual(cardLayout.cardLeft);
  expect.soft(cardLayout.actionsRight).toBeLessThanOrEqual(cardLayout.cardRight);
  expect.soft(cardLayout.modeButtons).toHaveLength(4);
  expect.soft(cardLayout.actionButtons.length).toBeGreaterThan(0);
  for (const button of [...cardLayout.modeButtons, ...cardLayout.actionButtons]) {
    assertInsideCardAndViewport(button);
  }
  expect.soft(overviewOverflow.documentScrollWidth).toBeLessThanOrEqual(overviewOverflow.documentClientWidth);
  expect.soft(overviewOverflow.bodyScrollWidth).toBeLessThanOrEqual(overviewOverflow.bodyClientWidth);

  const savedAssignments = await page.evaluate(async ({ firstName, secondName }) => {
    const [instancesResponse, packagesResponse] = await Promise.all([
      fetch("/api/instances"),
      fetch("/api/packages"),
    ]);
    const instances = await instancesResponse.json();
    const packages = await packagesResponse.json();
    const packageName = (id: string) =>
      packages.find((candidate: any) => (candidate.id ?? candidate.ID) === id)
        ?.filename;
    const snapshot = (name: string) => {
      const item = instances.find(
        (candidate: any) => (candidate.name ?? candidate.Name) === name,
      );
      const selected = item.package_id ?? item.SelectedPackageID;
      const applied = item.applied_package_id ?? item.PackageVersion;
      return {
        id: item.id ?? item.ID,
        extraArgs: item.extra_args ?? item.ExtraArgs,
        selected,
        applied,
        selectedName: packageName(selected),
        appliedName: packageName(applied),
      };
    };
    return { first: snapshot(firstName), second: snapshot(secondName) };
  }, { firstName: instanceName, secondName: secondInstanceName });
  const initiallySaved = savedAssignments.first;
  expect(initiallySaved.extraArgs).toBe(initialExtraArgs);
  expect(initiallySaved.selected).toBe(initiallySaved.applied);
  expect(initiallySaved.selectedName).toBe(packageAName);
  expect(initiallySaved.appliedName).toBe(packageAName);
  expect(savedAssignments.second.extraArgs).toBe(secondExtraArgs);
  expect(savedAssignments.second.selected).toBe(savedAssignments.second.applied);
  expect(savedAssignments.second.selectedName).toBe(packageBName);
  expect(savedAssignments.second.appliedName).toBe(packageBName);

  await card
    .getByRole("button", { name: `配置 ${instanceName}` })
    .click();
  await page.getByLabel("插件包").selectOption({
    label: `${packageBName} · ${packageBName}`,
  });
  await page.getByLabel("额外 SRCDS 启动项").fill(changedExtraArgs);
  await expect(page.getByLabel("启动指令预览")).toContainText(
    changedExtraArgs,
  );
  const reconfigureJob = await captureJob(
    page,
    `/api/instances/${initiallySaved.id}`,
    () => page.getByRole("button", { name: "保存并应用" }).click(),
    "PUT",
  );
  expect(reconfigureJob.Type).toBe("reconfigure");
  card = instanceCard(instanceName);
  await expect(card).toContainText(packageBName);
  await expect(card).toContainText("待应用");
  await waitForJob(page, reconfigureJob.ID);
  await page.reload();
  card = instanceCard(instanceName);
  await expect(card).toContainText(packageBName);
  await expect(card).not.toContainText("待应用");

  const changed = await page.evaluate(
    async ({ id, name }) => {
      const [instancesResponse, jobsResponse] = await Promise.all([
        fetch("/api/instances"),
        fetch("/api/jobs"),
      ]);
      const instances = await instancesResponse.json();
      const jobs = await jobsResponse.json();
      const item = instances.find(
        (candidate: any) => (candidate.name ?? candidate.Name) === name,
      );
      return {
        extraArgs: item.extra_args ?? item.ExtraArgs,
        selected: item.package_id ?? item.SelectedPackageID,
        applied: item.applied_package_id ?? item.PackageVersion,
        reconfigureJobs: jobs.filter(
          (job: any) =>
            job.InstanceID === id && job.Type === "reconfigure",
        ).length,
      };
    },
    { id: initiallySaved.id, name: instanceName },
  );
  expect(changed.extraArgs).toBe(changedExtraArgs);
  expect(changed.selected).toBe(changed.applied);
  expect(changed.reconfigureJobs).toBe(1);

  await page.reload();
  await expect(page.getByRole("heading", { name: "服务器作战室" })).toBeVisible();
  card = instanceCard(instanceName);
  await expect(card).toContainText("运行中");

  await page.getByRole("button", { name: "私有文件" }).click();
  await expect(page.getByRole("heading", { name: "私有文件", exact: true })).toBeVisible();
  await page.getByLabel("目标实例").selectOption(initiallySaved.id);

  await answerDialogs(page, ["cfg"]);
  await page.getByRole("button", { name: "新建目录" }).click();
  await answerDialogs(page, ["cfg/seeded.cfg"]);
  await page.getByRole("button", { name: "新建文件" }).click();
  await page.getByLabel("文件内容").fill("private override\n");
  await page.getByRole("button", { name: "保存到暂存区" }).click();
  await expect(page.getByLabel("暂存更改状态")).toContainText("1 项更改未应用");
  await expect(page.getByLabel("暂存更改状态")).toContainText("新增 1");

  const firstPrivateApply = await captureJob(page, "/private/apply", () =>
    page.getByRole("button", { name: "应用更改" }).click(),
  );
  await waitForJob(page, firstPrivateApply.ID);
  await expect(page.getByRole("status")).toContainText("私有文件已应用");
  await page.reload();
  await page.getByRole("button", { name: "私有文件" }).click();
  await page.getByLabel("目标实例").selectOption(initiallySaved.id);
  let tree = await privateTree(page, mobile);
  await tree.getByRole("treeitem", { name: "cfg", exact: true }).click();
  await expect(tree.getByRole("treeitem", { name: "seeded.cfg" })).toBeVisible();

  await answerDialogs(page, ["cfg/renamed.cfg", false]);
  await tree.getByLabel("移动 seeded.cfg").click();
  await expect(tree.getByRole("treeitem", { name: "renamed.cfg" })).toBeVisible();
  await answerDialogs(page, ["cfg/seeded.cfg", false]);
  await tree.getByLabel("移动 renamed.cfg").click();

  const binary = Buffer.from([0, 1, 2, 3, 255, 128]);
  await tree.getByRole("treeitem", { name: "cfg", exact: true }).click();
  await closePrivateTree(page, mobile);
  const privateUploadRequest = page.waitForResponse((response) =>
    response.request().method() === "POST" && response.url().endsWith("/private/uploads"),
  );
  await page.getByLabel("上传文件").setInputFiles({
    name: "binary.bin",
    mimeType: "application/octet-stream",
    buffer: binary,
  });
  const privateUploadResponse = await privateUploadRequest;
  expect(privateUploadResponse.status(), await privateUploadResponse.text()).toBe(201);
  await expect(page.getByText("上传完成 · 100%", { exact: true })).toBeVisible();
  tree = await privateTree(page, mobile);
  await tree.getByRole("treeitem", { name: "cfg", exact: true }).click();
  await tree.getByRole("treeitem", { name: "binary.bin" }).click();
  await closePrivateTree(page, mobile);
  const downloadPromise = page.waitForEvent("download");
  await page.getByRole("link", { name: "下载 binary.bin" }).click();
  const download = await downloadPromise;
  expect(await download.createReadStream().then(async (stream) => {
    const chunks: Buffer[] = [];
    for await (const chunk of stream) chunks.push(Buffer.from(chunk));
    return Buffer.concat(chunks);
  })).toEqual(binary);

  const secondPrivateApply = await captureJob(page, "/private/apply", () =>
    page.getByRole("button", { name: "应用更改" }).click(),
  );
  await waitForJob(page, secondPrivateApply.ID);
  const restorableSnapshotID = await page.evaluate(async (id) => {
    const response = await fetch(`/api/instances/${id}/private/snapshots`);
    const values = await response.json() as Array<{ id: string }>;
    if (!response.ok || !values[0]?.id) throw new Error("expected applied private snapshot");
    return values[0].id;
  }, initiallySaved.id);
  tree = await privateTree(page, mobile);
  await expect(tree.getByRole("treeitem", { name: "seeded.cfg" })).toBeVisible();
  await answerDialogs(page, [true]);
  await tree.getByLabel("删除 seeded.cfg").click();
  await closePrivateTree(page, mobile);
  const deletePrivateApply = await captureJob(page, "/private/apply", () =>
    page.getByRole("button", { name: "应用更改" }).click(),
  );
  await waitForJob(page, deletePrivateApply.ID);
  await page.getByRole("button", { name: "刷新" }).click();
  tree = await privateTree(page, mobile);
  const deletedCfg = tree.getByRole("treeitem", { name: "cfg", exact: true });
  if (await deletedCfg.getAttribute("aria-expanded") !== "true") await deletedCfg.click();
  await expect(tree.getByRole("treeitem", { name: "seeded.cfg" })).toHaveCount(0);
  await expect(tree.getByRole("treeitem", { name: "binary.bin" })).toBeVisible();
  await closePrivateTree(page, mobile);
  const lowerDiagnostic = await page.evaluate(async (id) => {
    const response = await fetch(`/__e2e/private-lower?id=${encodeURIComponent(id)}&path=cfg/seeded.cfg`);
    return { status: response.status, body: await response.text() };
  }, initiallySaved.id);
  expect(lowerDiagnostic).toEqual({ status: 200, body: "fixture lower layer\n" });

  await page.getByRole("button", { name: "历史快照" }).click();
  const snapshots = page.getByRole("dialog", { name: "历史快照" });
  const snapshotIndex = await page.evaluate(async ({ id, snapshotID }) => {
    const response = await fetch(`/api/instances/${id}/private/snapshots`);
    const values = await response.json() as Array<{ id: string }>;
    return values.findIndex((snapshot) => snapshot.id === snapshotID);
  }, { id: initiallySaved.id, snapshotID: restorableSnapshotID });
  expect(snapshotIndex).toBeGreaterThanOrEqual(0);
  await expect.poll(() => snapshots.locator(".private-snapshot-row").count()).toBeGreaterThan(snapshotIndex);
  await answerDialogs(page, [true]);
  await snapshots.getByRole("button", { name: /^恢复 / }).nth(snapshotIndex).click();
  await expect(page.getByText("快照已恢复到暂存区", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "刷新" }).click();
  await expect(page.getByLabel("暂存更改状态")).toContainText("更改未应用");
  tree = await privateTree(page, mobile);
  const restoredCfg = tree.getByRole("treeitem", { name: "cfg", exact: true });
  if (await restoredCfg.getAttribute("aria-expanded") !== "true") await restoredCfg.click();
  await expect(tree.getByRole("treeitem", { name: "seeded.cfg" })).toBeVisible();
  await expect(tree.getByRole("treeitem", { name: "binary.bin" })).toBeVisible();
  await tree.getByLabel("编辑 seeded.cfg").click();
  await expect(page.getByLabel("文件内容")).toHaveValue("private override\n");
  const restorePrivateApply = await captureJob(page, "/private/apply", () =>
    page.getByRole("button", { name: "应用更改" }).click(),
  );
  await waitForJob(page, restorePrivateApply.ID);

  const archiveDownloadPromise = page.waitForEvent("download");
  await page.getByRole("button", { name: "导出 ZIP" }).click();
  const archiveDownload = await archiveDownloadPromise;
  expect(archiveDownload.suggestedFilename()).toBe(
    `private-files-${initiallySaved.id}.zip`,
  );
  const exportedArchive = await archiveDownload.createReadStream().then(
    async (stream) => {
      const chunks: Buffer[] = [];
      for await (const chunk of stream) chunks.push(Buffer.from(chunk));
      return Buffer.concat(chunks);
    },
  );
  expect(exportedArchive.byteLength).toBeGreaterThan(0);
  expect(exportedArchive.subarray(0, 2).toString("ascii")).toBe("PK");

  const importDialogPromise = page.waitForEvent("dialog");
  const importAction = page.getByLabel("导入 ZIP").setInputFiles({
    name: "replacement.zip",
    mimeType: "application/zip",
    buffer: privateReplacementZip,
  });
  const importDialog = await importDialogPromise;
  expect(importDialog.message()).toContain(
    "ZIP 中不存在的现有文件和未应用更改将被删除，不会保留",
  );
  expect(importDialog.message()).toContain("历史应用快照不受影响");
  expect(importDialog.message()).toContain("不会自动应用到游戏目录");
  await importDialog.accept();
  await importAction;
  await expect(
    page.getByText("工作区已完全替换，请检查差异后应用更改。", {
      exact: true,
    }),
  ).toBeVisible();
  await expect(page.getByLabel("暂存更改状态")).toContainText("3 项更改未应用");
  tree = await privateTree(page, mobile);
  await expect(tree.getByRole("treeitem", { name: "cfg", exact: true })).toHaveCount(0);
  await expect(tree.getByRole("treeitem", { name: "seeded.cfg" })).toHaveCount(0);
  await expect(tree.getByRole("treeitem", { name: "binary.bin" })).toHaveCount(0);
  await tree.getByRole("treeitem", { name: "imported", exact: true }).click();
  await expect(tree.getByRole("treeitem", { name: "new.cfg" })).toBeVisible();
  await closePrivateTree(page, mobile);
  const stagedImportLower = await page.evaluate(async (id) => {
    const request = async (path: string) => {
      const response = await fetch(
        `/__e2e/private-lower?id=${encodeURIComponent(id)}&path=${encodeURIComponent(path)}`,
      );
      return { status: response.status, body: await response.text() };
    };
    return {
      previouslyApplied: await request("cfg/seeded.cfg"),
      stagedOnly: await request("imported/new.cfg"),
    };
  }, initiallySaved.id);
  expect(stagedImportLower.previouslyApplied).toEqual({
    status: 200,
    body: "private override\n",
  });
  expect(stagedImportLower.stagedOnly.status).toBe(404);

  const privateLayout = await page.locator(".private-files-page").evaluate((root) => {
    const layout = root.querySelector(".private-files-layout")!.getBoundingClientRect();
    const workspace = root.querySelector(".private-workspace")!.getBoundingClientRect();
    const status = root.querySelector(".private-change-bar")!.getBoundingClientRect();
    return { layout, workspace, status, pageWidth: document.documentElement.scrollWidth, viewportWidth: window.innerWidth };
  });
  expect.soft(privateLayout.pageWidth).toBeLessThanOrEqual(privateLayout.viewportWidth);
  expect.soft(privateLayout.layout.right).toBeLessThanOrEqual(privateLayout.viewportWidth);
  expect.soft(privateLayout.workspace.right).toBeLessThanOrEqual(privateLayout.viewportWidth);
  expect.soft(privateLayout.status.right).toBeLessThanOrEqual(privateLayout.viewportWidth);
  if (mobile) {
    const drawerTrigger = page.getByRole("button", { name: "打开文件树" });
    await drawerTrigger.click();
    const drawer = page.getByRole("dialog", { name: "私有文件目录" });
    await expect(drawer).toBeVisible();
    await expect(page.getByRole("button", { name: "关闭文件树" })).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(drawerTrigger).toBeFocused();
  }

  await page.getByRole("button", { name: "总览" }).click();
  card = instanceCard(instanceName);

  await card.getByRole("button", { name: "控制台" }).click();
  const consoleOutput = page.locator(".terminal-modal pre");
  await expect(consoleOutput).toContainText("fixture overflow 119");
  await expect.poll(async () => {
    const position = await consolePosition(page);
    return Math.abs(position.top - position.bottom);
  }).toBeLessThanOrEqual(2);
  expect.soft((await consolePosition(page)).scrollWidth).toBeLessThanOrEqual((await consolePosition(page)).clientWidth);

  await consoleOutput.evaluate((output) => {
    output.scrollTop = 0;
    output.dispatchEvent(new Event("scroll", { bubbles: true }));
  });
  await expect.poll(async () => (await consolePosition(page)).top).toBeLessThanOrEqual(2);
  await page.evaluate(async (id) => {
    await fetch(`/__e2e/console-output?id=${encodeURIComponent(id)}`, { method: "POST", body: "async held\n" });
  }, initiallySaved.id);
  await expect(consoleOutput).toContainText("async held");
  expect((await consolePosition(page)).top).toBeLessThanOrEqual(2);

  await consoleOutput.hover();
  await page.mouse.wheel(0, 100_000);
  await expect.poll(async () => {
    const position = await consolePosition(page);
    return Math.abs(position.top - position.bottom);
  }).toBeLessThanOrEqual(2);
  await page.evaluate(async (id) => {
    await fetch(`/__e2e/console-output?id=${encodeURIComponent(id)}`, { method: "POST", body: "async followed\n" });
  }, initiallySaved.id);
  await expect(consoleOutput).toContainText("async followed");
  await expect.poll(async () => {
    const position = await consolePosition(page);
    return Math.abs(position.top - position.bottom);
  }).toBeLessThanOrEqual(2);

  await consoleOutput.evaluate((output) => {
    output.scrollTop = 0;
    output.dispatchEvent(new Event("scroll", { bubbles: true }));
  });
  await expect.poll(async () => (await consolePosition(page)).top).toBeLessThanOrEqual(2);
  await page.locator(".terminal-modal input").fill("status");
  await page.locator(".terminal-modal").getByRole("button", { name: "发送" }).click();
  await expect(page.locator(".terminal-modal pre")).toContainText("echo:status");
  await expect.poll(async () => {
    const position = await consolePosition(page);
    return Math.abs(position.top - position.bottom);
  }).toBeLessThanOrEqual(2);
  await page.locator(".terminal-head button").click();
  await card.getByRole("button", { name: "控制台" }).click();
  await expect(page.locator(".terminal-modal pre")).toContainText("fixture console ready");
  await page.locator(".terminal-head button").click();

  await card.getByRole("button", { name: "玩家" }).click();
  await expect(page.getByText("Fixture Player")).toBeVisible();
  const playersDialog = page.getByRole("dialog", { name: instanceName });
  await expect(playersDialog.getByRole("region", { name: "对局摘要" })).toContainText("c2m1_highway");
  await expect(playersDialog).toContainText("Fixture Host");
  await expect(playersDialog).toContainText("STEAM_1:0:42");
  await expect(playersDialog).toContainText("01:30");
  await expect(playersDialog).toContainText("45 ms");
  await expect(playersDialog).toContainText("0%");
  await page.getByRole("button", { name: "踢出" }).click();
  const confirmKick = page.getByRole("button", { name: "确认踢出" });
  const cancel = page.getByRole("button", { name: "取消" });
  await expect(confirmKick).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(cancel).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(confirmKick).toBeFocused();
  const playerJob = await captureJob(page, "/players/7/actions", () =>
    confirmKick.click(),
  );
  await waitForJob(page, playerJob.ID);
  await page.getByRole("button", { name: "关闭玩家列表" }).click();

  await page.getByRole("button", { name: "内容仓库" }).click();
  const vpk = Buffer.alloc(9 * 1024 * 1024, mobile ? 0x4d : 0x44);
  await page.locator('input[accept=".vpk"]').setInputFiles({
    name: `fixture-${suffix.toLowerCase()}.vpk`,
    mimeType: "application/octet-stream",
    buffer: vpk,
  });
  await expect(page.getByRole("status")).toContainText("VPK 上传完成");
  expect(vpkChunks).toBe(2);

  const packageRow = page
    .locator(".data-row")
    .filter({ hasText: packageBName });
  await packageRow.getByRole("button", { name: "完整更新" }).click();
  const confirmFull = page.getByRole("button", { name: "确认完整更新" });
  await expect(confirmFull).toBeFocused();
  const fullJob = await captureJob(page, "/updates", () => confirmFull.click());
  await waitForJob(page, fullJob.ID);

  await page.getByRole("button", { name: "计划任务" }).click();
  await page.getByLabel("Cron").fill(mobile ? "15 4 * * *" : "0 4 * * *");
  await page.getByRole("button", { name: "保存计划" }).click();
  await expect(page.getByRole("status")).toContainText("计划已保存");
  await expect(page.getByText(mobile ? "15 4 * * *" : "0 4 * * *")).toBeVisible();

  await page.getByRole("button", { name: "任务", exact: true }).click();
  await expect(page.getByText("SSE / LIVE")).toBeVisible();
  await expect(page.getByText("interrupted", { exact: true })).toBeVisible();
  await expect(
    page.locator(".job-row").filter({ hasText: "fixture_success" }),
  ).toContainText("succeeded");
  await expect(
    page.locator(".job-row").filter({ hasText: "fixture_failure" }),
  ).toContainText("deterministic fixture failure");
  await expect(
    page.locator(".job-row").filter({ hasText: "fixture_slow" }),
  ).toContainText("deterministic slow job");
  await expect(
    page.locator(".job-row").filter({ hasText: "fixture_recovery" }),
  ).toContainText("recovered after fixture restart");
  await expect(
    page.locator(".job-row").filter({ hasText: fullJob.ID.slice(0, 8) }),
  ).toContainText("succeeded");
  await expect(
    page.locator(".job-row").filter({ hasText: reconfigureJob.ID.slice(0, 8) }),
  ).toContainText("succeeded");
  await expect(page.locator(".job-row").first()).toBeVisible();

  const failedJob = page.getByRole("button", {
    name: "查看 fixture_failure 任务日志，任务 ID fixture-failure",
  });
  await failedJob.click();
  await expect(failedJob).toHaveAttribute("aria-expanded", "true");
  const failedLog = page.getByRole("region", {
    name: "fixture_failure 任务日志",
  });
  await expect(failedLog).toContainText("deterministic fixture failure");
  await expect(failedLog).toContainText("进入队列");
  await expect(failedLog).toContainText("开始执行");
  await expect(failedLog).toContainText("执行进度");
  await expect(failedLog).toContainText("任务失败");
  await expect(failedLog).toContainText("执行用时 2分18秒");
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("recent-job-logs.png"),
    fullPage: true,
  });

  const latestJob = page.locator(".activity");
  await expect.soft(latestJob).toContainText("任务已成功完成");
  await expect.soft(latestJob).not.toContainText("后台任务持久化执行中");

  if (mobile) {
    const layout = await page.evaluate(() => {
      const main = document.querySelector("main")!.getBoundingClientRect();
      const navigation = document.querySelector("aside")!.getBoundingClientRect();
      const mainElement = document.querySelector("main")!;
      return {
        mainBottom: main.bottom,
        navigationTop: navigation.top,
        mainClientHeight: mainElement.clientHeight,
        mainScrollHeight: mainElement.scrollHeight,
      };
    });
    expect.soft(layout.mainBottom).toBeLessThanOrEqual(layout.navigationTop + 1);
    expect.soft(layout.mainScrollHeight).toBeGreaterThan(layout.mainClientHeight);
    const navigationFits = await page.locator("nav button").evaluateAll((buttons) =>
      buttons.every(
        (button) =>
          button.scrollWidth <= button.clientWidth &&
          button.scrollHeight <= button.clientHeight,
      ),
    );
    expect.soft(navigationFits).toBe(true);
  }

  const fitsViewport = await page.evaluate(
    () => document.documentElement.scrollWidth <= window.innerWidth,
  );
  expect(fitsViewport).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath(`${testInfo.project.name}-journey.png`),
    fullPage: true,
  });

  await page.getByRole("button", { name: "系统设置" }).click();
  const retentionLimit = page.getByRole("spinbutton", {
    name: "成功任务保留数量",
  });
  await expect(retentionLimit).toHaveValue("40");
  await retentionLimit.fill("25");
  await page
    .getByRole("button", { name: "保存任务记录设置" })
    .click();
  await expect(page.getByRole("status")).toContainText(
    "任务记录设置已保存",
  );
  await expect(retentionLimit).toHaveValue("25");
  await expect
    .poll(() =>
      page.evaluate(async () => {
        const response = await fetch("/api/settings/jobs");
        return (await response.json()).successful_job_limit;
      }),
    )
    .toBe(25);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
});

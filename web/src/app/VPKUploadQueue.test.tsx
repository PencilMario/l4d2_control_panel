import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { VPKUploadQueue, formatBytes, formatDuration, formatSpeed } from "./VPKUploadQueue";

describe("VPKUploadQueue", () => {
  it("shows uploaded bytes, current speed, average speed, and ETA", () => {
    render(<VPKUploadQueue tasks={[{ id: "1", name: "map.vpk", mode: "direct", sourceSize: 16 * 1024 * 1024, size: 16 * 1024 * 1024, blob: new Blob(), status: "uploading", offset: 8 * 1024 * 1024, speed: 2 * 1024 * 1024, averageSpeed: 1024 * 1024, etaSeconds: 4 }]} onRetry={() => {}} onCancel={() => {}} />);
    expect(screen.getByRole("heading", { name: "正在处理的上传队列 (1)" })).toBeVisible();
    expect(screen.getByRole("list", { name: "上传任务" })).toBeVisible();
    expect(screen.queryByText("刷新页面后会自动恢复")).not.toBeInTheDocument();
    expect(screen.getAllByText(/8\.0 MiB \/ 16\.0 MiB/)).toHaveLength(2);
    expect(screen.getByText(/实时 2\.0 MiB\/s/)).toBeVisible();
    expect(screen.getByText(/平均 1\.0 MiB\/s/)).toBeVisible();
    expect(screen.getByText(/剩余 4 秒/)).toBeVisible();
    expect(screen.getByRole("progressbar", { name: "map.vpk 上传进度" })).toHaveClass("vpk-upload-progress-track");
    expect(screen.getByRole("progressbar", { name: "map.vpk 上传进度" })).toHaveAttribute("aria-valuenow", "50");
  });
  it("formats byte and duration metrics", () => {
    expect(formatBytes(1024)).toBe("1.0 KiB");
    expect(formatSpeed(1024 * 1024)).toBe("1.0 MiB/s");
    expect(formatDuration(65)).toBe("1 分 5 秒");
  });
});

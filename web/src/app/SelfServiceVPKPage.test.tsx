import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { SelfServiceVPKPage } from "./SelfServiceVPKPage";

const queue = vi.hoisted(() => ({ enqueue: vi.fn(), start: vi.fn(async (onChange: (items: unknown[]) => void) => { onChange([]); return () => {}; }) }));
vi.mock("../vpk/uploadQueue", async (load) => {
  const actual = await load<typeof import("../vpk/uploadQueue")>();
  return { ...actual, enqueueVPKUploads: queue.enqueue, startVPKUploadQueue: queue.start, cancelVPKUpload: vi.fn(), retryVPKUpload: vi.fn() };
});

describe("SelfServiceVPKPage", () => {
  beforeEach(() => { vi.restoreAllMocks(); queue.enqueue.mockReset(); queue.start.mockClear(); });

  it("shows the disabled state without exposing upload controls", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json({ enabled: false, password_required: false, auto_delete: false }));
    render(<SelfServiceVPKPage />);
    expect(await screen.findByText("自助传图暂未开放")).toBeInTheDocument();
    expect(screen.queryByLabelText("选择 VPK 文件")).not.toBeInTheDocument();
  });

  it("unlocks, lists self-service files, and queues cleanup uploads", async () => {
	let authorized = false;
    vi.spyOn(globalThis, "fetch").mockImplementation(async (input, init) => {
      const path = String(input);
      if (path.endsWith("/status")) return Response.json({ enabled: true, password_required: true, auto_delete: true });
      if (path.endsWith("/authorize") && init?.method === "POST") { authorized = true; return new Response(null, { status: 204 }); }
      if (path.includes("/api/self-service/vpk?")) return authorized ? Response.json({ items: [{ name: "map.vpk", size: 1024, uploaded_at: "2026-08-02T12:00:00Z", expires_at: "2026-08-09T12:00:00Z" }], total: 1, limit: 20, offset: 0, auto_delete: true }) : Response.json({ error: { message: "authorization required" } }, { status: 401 });
      return new Response(null, { status: 404 });
    });
    render(<SelfServiceVPKPage />);
    await userEvent.type(await screen.findByLabelText("访问密码"), "maps");
    await userEvent.click(screen.getByRole("button", { name: "进入自助传图" }));
    expect(await screen.findByText("map.vpk")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "上传前清理" }));
    const file = new File(["vpk"], "new-map.vpk", { type: "application/octet-stream" });
    await userEvent.upload(screen.getByLabelText("选择 VPK 文件"), file);
    await waitFor(() => expect(queue.enqueue).toHaveBeenCalledWith([{ file, mode: "clean" }]));
  });
});

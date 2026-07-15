import {
  act,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { StrictMode } from "react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { App, mergePerformanceHistory, prunePerformanceHistory, type Instance } from "./App";
const instance: Instance = {
  id: "1",
  name: "深夜战役",
  actual_state: "running",
  game_port: 27015,
  sourcetv_port: 27020,
  plugin_ports: [27021],
  start_map: "c2m1_highway",
  game_mode: "coop",
  tickrate: 100,
  max_players: 8,
  extra_args: `-strictportbind`,
  package_id: "package-a",
  applied_package_id: "package-a",
  players: 4,
  cpu: 31,
  memory: 2.4,
};
const apiInstance = {
  ID: "1",
  NodeID: "local",
  Name: "深夜战役",
  ContainerID: "container-1",
  GamePort: 27015,
  sourcetv_port: 27020,
  plugin_ports: [27021],
  StartMap: "c2m1_highway",
  GameMode: "coop",
  Tickrate: 100,
  MaxPlayers: 8,
  extra_args: "-strictportbind",
  RuntimeImage: "runtime",
  applied_package_id: "package-a",
  package_id: "package-a",
  DesiredState: "running",
  ActualState: "running",
  CreatedAt: "2026-07-15T00:00:00Z",
  UpdatedAt: "2026-07-15T00:00:00Z",
};
const stoppedOverview = {
  actual_state: "stopped",
  container_running: false,
  map: "",
  players: null,
  max_players: null,
  cpu_percent: null,
  memory_bytes: null,
  issues: [],
};
const runningZeroOverview = {
  actual_state: "running",
  container_running: true,
  map: "c5m1_waterfront",
  players: 0,
  max_players: 8,
  cpu_percent: 0,
  memory_bytes: 0,
  issues: [],
};
function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}
afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});
describe("App", () => {
  it("keeps console websocket chunks and follows after sending a command", async () => {
    const sockets: FakeWebSocket[] = [];
    class FakeWebSocket {
      binaryType = "";
      onmessage: ((event: MessageEvent) => void) | null = null;
      send = vi.fn();
      close = vi.fn();
      constructor(public url: string) { sockets.push(this); }
    }
    vi.stubGlobal("WebSocket", FakeWebSocket);
    let nextFrame = 1;
    const frames = new Map<number, FrameRequestCallback>();
    vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
      const id = nextFrame++;
      frames.set(id, callback);
      return id;
    });
    vi.stubGlobal("cancelAnimationFrame", (id: number) => frames.delete(id));
    const flushFrames = () => {
      const pending = [...frames.entries()];
      frames.clear();
      pending.forEach(([id, callback]) => callback(id));
    };
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "控制台" }));
    const output = document.querySelector(".terminal-modal pre") as HTMLPreElement;
    let scrollTop = 0;
    Object.defineProperties(output, {
      scrollHeight: { configurable: true, get: () => 600 },
      clientHeight: { configurable: true, get: () => 100 },
      scrollTop: {
        configurable: true,
        get: () => scrollTop,
        set: (value: number) => { scrollTop = value; },
      },
    });
    act(() => sockets[0].onmessage?.({ data: "ready\n" } as MessageEvent));
    act(() => flushFrames());
    expect(output).toHaveTextContent("ready");
    expect(scrollTop).toBe(600);

    await userEvent.type(screen.getByRole("textbox"), "status");
    await userEvent.click(screen.getByRole("button", { name: "发送" }));
    act(() => flushFrames());
    expect(sockets[0].send).toHaveBeenCalledWith("status\n");
    expect(scrollTop).toBe(600);
    expect(sockets[0].url).toContain("/api/instances/1/console");

    act(() => {
      for (let index = 0; index <= 500; index += 1) {
        sockets[0].onmessage?.({ data: `[${index}]\n` } as MessageEvent);
      }
    });
    act(() => flushFrames());
    expect(output).not.toHaveTextContent("ready");
    expect(output).not.toHaveTextContent("[0]");
    expect(output).toHaveTextContent("[1]");
    expect(output).toHaveTextContent("[500]");
    expect(scrollTop).toBe(600);
  });

  it("deduplicates, sorts and caps performance history", () => {
    const points = Array.from({ length: 721 }, (_, index) => ({
      at: new Date(Date.UTC(2026, 6, 15, 0, 0, index)).toISOString(),
      run_id: "run-1",
      cpu_percent: index,
      memory_percent: null,
      network_rx_bytes_per_sec: null,
      network_tx_bytes_per_sec: null,
      block_read_bytes_per_sec: null,
      block_write_bytes_per_sec: null,
    }));
    const merged = mergePerformanceHistory(points, [{ ...points[720], cpu_percent: 999 }]);
    expect(merged).toHaveLength(720);
    expect(merged[0].cpu_percent).toBe(1);
    expect(merged[719].cpu_percent).toBe(999);
  });
  it("orders history by instant, keeps stable ties, ignores invalid timestamps and caps newest points", () => {
    const point = (at: string, run_id: string, cpu_percent: number) => ({
      at,
      run_id,
      cpu_percent,
      memory_percent: null,
      network_rx_bytes_per_sec: null,
      network_tx_bytes_per_sec: null,
      block_read_bytes_per_sec: null,
      block_write_bytes_per_sec: null,
    });
    const offsets = mergePerformanceHistory([], [
      point("2026-07-15T12:00:00Z", "tie-first", 1),
      point("2026-07-15T13:00:00+01:00", "tie-second", 2),
      point("2026-07-15T13:00:00+02:00", "earlier", 3),
      point("not-a-timestamp", "invalid", 4),
    ]);
    expect(offsets.map((item) => item.run_id)).toEqual([
      "earlier",
      "tie-first",
      "tie-second",
    ]);

    const many = Array.from({ length: 721 }, (_, index) =>
      point(new Date(Date.UTC(2026, 6, 15, 0, 0, index)).toISOString(), `run-${index}`, index),
    ).reverse();
    const capped = mergePerformanceHistory([], many);
    expect(capped).toHaveLength(720);
    expect(capped[0].cpu_percent).toBe(1);
    expect(capped[719].cpu_percent).toBe(720);
  });
  it("removes histories for deleted instances", () => {
    const point = { at: "2026-07-15T12:00:00Z", run_id: "run-1", cpu_percent: null, memory_percent: null, network_rx_bytes_per_sec: null, network_tx_bytes_per_sec: null, block_read_bytes_per_sec: null, block_write_bytes_per_sec: null };
    expect(prunePerformanceHistory({ present: [point], deleted: [point] }, new Set(["present"]))).toEqual({ present: [point] });
  });

  it("fetches history once and appends overview samples on the existing poll", async () => {
    const intervalSpy = vi.spyOn(window, "setInterval");
    const initialHistory = deferred<Response>();
    let overviewIndex = 0;
    const calls: string[] = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      calls.push(path);
      const value = path === "/api/session" ? { authenticated: true }
        : path === "/api/instances" ? [apiInstance]
        : path === "/api/instances/1/performance-history" ? null
        : path === "/api/instances/1/overview" ? { ...runningZeroOverview, sampled_at: overviewIndex++ === 0 ? "2026-07-15T12:00:05Z" : "2026-07-15T12:00:10Z", run_id: "run-1", container_running_known: true, image_size_bytes: 5 * 1024 ** 3, memory_limit_bytes: 1024, memory_percent: 0, network_rx_bytes_per_sec: 0, network_tx_bytes_per_sec: 0, network_rx_bytes: 0, network_tx_bytes: 0, block_read_bytes_per_sec: 0, block_write_bytes_per_sec: 0, block_read_bytes: 0, block_write_bytes: 0, pids: 0, uptime_seconds: 0, a2s_latency_ms: 0 }
        : path === "/api/packages" ? [] : { ok: true };
      if (path === "/api/instances/1/performance-history") return initialHistory.promise;
      return new Response(JSON.stringify(value), { status: 200, headers: { "Content-Type": "application/json" } });
    }));

    render(<App />);
    await waitFor(() => expect(intervalSpy.mock.calls.some(([, timeout]) => timeout === 5_000)).toBe(true));
    const refresh = intervalSpy.mock.calls.find(([, timeout]) => timeout === 5_000)![0] as () => void;
    initialHistory.resolve(new Response(JSON.stringify([{ at: "2026-07-15T12:00:00Z", run_id: "run-1", cpu_percent: 1, memory_percent: 2, network_rx_bytes_per_sec: null, network_tx_bytes_per_sec: null, block_read_bytes_per_sec: null, block_write_bytes_per_sec: null }]), { status: 200, headers: { "Content-Type": "application/json" } }));
    expect(await screen.findByTestId("performance-chart")).toHaveAttribute("data-point-count", "2");
    expect(screen.getByText("5 GiB")).toBeInTheDocument();
    await act(async () => refresh());
    await waitFor(() => expect(screen.getByTestId("performance-chart")).toHaveAttribute("data-point-count", "3"));
    expect(calls.filter((path) => path.endsWith("/performance-history"))).toHaveLength(1);
  });
  it("keeps live overview updates flowing while one history bootstrap is hung", async () => {
    const intervalSpy = vi.spyOn(window, "setInterval");
    const historyResponse = deferred<Response>();
    let historyCalls = 0;
    let overviewCalls = 0;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      if (path === "/api/session") return new Response('{"authenticated":true}', { status: 200, headers: { "Content-Type": "application/json" } });
      if (path === "/api/instances") return new Response(JSON.stringify([apiInstance]), { status: 200, headers: { "Content-Type": "application/json" } });
      if (path.endsWith("/performance-history")) {
        historyCalls += 1;
        return historyResponse.promise;
      }
      if (path.endsWith("/overview")) {
        overviewCalls += 1;
        return new Response(JSON.stringify({ ...runningZeroOverview, cpu_percent: overviewCalls * 10, sampled_at: `2026-07-15T12:00:${String(overviewCalls * 5).padStart(2, "0")}Z`, run_id: "run-1", container_running_known: true }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      const value = path === "/api/packages" ? [] : { ok: true };
      return new Response(JSON.stringify(value), { status: 200, headers: { "Content-Type": "application/json" } });
    }));

    render(<App />);
    expect(await screen.findByText("10%")).toBeInTheDocument();
    await waitFor(() => expect(intervalSpy.mock.calls.some(([, timeout]) => timeout === 5_000)).toBe(true));
    const refresh = intervalSpy.mock.calls.find(([, timeout]) => timeout === 5_000)![0] as () => void;
    await act(async () => refresh());
    expect(await screen.findByText("20%")).toBeInTheDocument();
    await act(async () => refresh());
    expect(await screen.findByText("30%")).toBeInTheDocument();
    expect(historyCalls).toBe(1);
    expect(screen.getByTestId("performance-chart")).toHaveAttribute("data-point-count", "3");

    historyResponse.resolve(new Response(JSON.stringify([{ at: "2026-07-15T12:00:00Z", run_id: "run-1", cpu_percent: 5, memory_percent: null, network_rx_bytes_per_sec: null, network_tx_bytes_per_sec: null, block_read_bytes_per_sec: null, block_write_bytes_per_sec: null }]), { status: 200, headers: { "Content-Type": "application/json" } }));
    await waitFor(() => expect(screen.getByTestId("performance-chart")).toHaveAttribute("data-point-count", "4"));
  });
  it("does not let an older poll restore deleted instances or stale history ownership", async () => {
    const intervalSpy = vi.spyOn(window, "setInterval");
    const oldOverview = deferred<Response>();
    const oldHistory = deferred<Response>();
    let instanceLists = 0;
    let historyCalls = 0;
    let oldHistorySignal: AbortSignal | null | undefined;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path === "/api/session") return new Response('{"authenticated":true}', { status: 200, headers: { "Content-Type": "application/json" } });
      if (path === "/api/instances") {
        instanceLists += 1;
        return new Response(JSON.stringify(instanceLists === 2 ? [] : [apiInstance]), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (path === "/api/instances/1/overview") {
        if (instanceLists === 1) return oldOverview.promise;
        return new Response(JSON.stringify({ ...runningZeroOverview, sampled_at: "2026-07-15T12:00:20Z", run_id: "run-new", container_running_known: true }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (path === "/api/instances/1/performance-history") {
        historyCalls += 1;
        if (historyCalls === 1) {
          oldHistorySignal = init?.signal;
          return oldHistory.promise;
        }
        return new Response("[]", { status: 200, headers: { "Content-Type": "application/json" } });
      }
      const value = path === "/api/packages" ? [] : { ok: true };
      return new Response(JSON.stringify(value), { status: 200, headers: { "Content-Type": "application/json" } });
    }));

    render(<App />);
    await waitFor(() => expect(historyCalls).toBe(1));
    await screen.findByRole("heading", { name: "服务器作战室" });
    await waitFor(() => expect(intervalSpy.mock.calls.some(([, timeout]) => timeout === 5_000)).toBe(true));
    const refresh = intervalSpy.mock.calls.find(([, timeout]) => timeout === 5_000)![0] as () => void;
    await act(async () => refresh());
    expect(await screen.findByText("尚无实例。创建第一个 Host 网络服务器。")).toBeInTheDocument();
    expect(oldHistorySignal?.aborted).toBe(true);

    oldOverview.resolve(new Response(JSON.stringify({ ...runningZeroOverview, sampled_at: "2026-07-15T12:00:05Z", run_id: "run-old", container_running_known: true }), { status: 200, headers: { "Content-Type": "application/json" } }));
    oldHistory.resolve(new Response(JSON.stringify([{ at: "2026-07-15T12:00:00Z", run_id: "run-old", cpu_percent: 99, memory_percent: null, network_rx_bytes_per_sec: null, network_tx_bytes_per_sec: null, block_read_bytes_per_sec: null, block_write_bytes_per_sec: null }]), { status: 200, headers: { "Content-Type": "application/json" } }));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    await waitFor(() => expect(screen.queryByText("深夜战役")).not.toBeInTheDocument());
    await waitFor(() => expect(screen.queryByTestId("performance-chart")).not.toBeInTheDocument());

    await act(async () => refresh());
    await waitFor(() => expect(instanceLists).toBe(3));
    await waitFor(() => expect(historyCalls).toBe(2));
    expect(await screen.findByText("深夜战役")).toBeInTheDocument();
    expect(await screen.findByTestId("performance-chart")).toHaveAttribute("data-point-count", "1");
  });
  it("keeps one history bootstrap across poll generations without stale deletion poisoning", async () => {
    const intervalSpy = vi.spyOn(window, "setInterval");
    const oldHistory = deferred<Response>();
    const stalledDeletion = deferred<Response>();
    let listCalls = 0;
    let historyCalls = 0;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      if (path === "/api/session") return new Response('{"authenticated":true}', { status: 200, headers: { "Content-Type": "application/json" } });
      if (path === "/api/instances") {
        listCalls += 1;
        if (listCalls === 2) return stalledDeletion.promise;
        return new Response(JSON.stringify([apiInstance]), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (path === "/api/instances/1/performance-history") {
        historyCalls += 1;
        if (historyCalls === 1) return oldHistory.promise;
        return new Response(JSON.stringify([{ at: "2026-07-15T12:00:10Z", run_id: "run-new", cpu_percent: 22, memory_percent: null, network_rx_bytes_per_sec: null, network_tx_bytes_per_sec: null, block_read_bytes_per_sec: null, block_write_bytes_per_sec: null }]), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (path === "/api/instances/1/overview") return new Response(JSON.stringify({ ...runningZeroOverview, sampled_at: "2026-07-15T12:00:15Z", run_id: "run-new", container_running_known: true }), { status: 200, headers: { "Content-Type": "application/json" } });
      const value = path === "/api/packages" ? [] : { ok: true };
      return new Response(JSON.stringify(value), { status: 200, headers: { "Content-Type": "application/json" } });
    }));

    render(<App />);
    await waitFor(() => expect(historyCalls).toBe(1));
    await screen.findByRole("heading", { name: "服务器作战室" });
    await waitFor(() => expect(intervalSpy.mock.calls.some(([, timeout]) => timeout === 5_000)).toBe(true));
    const refresh = intervalSpy.mock.calls.find(([, timeout]) => timeout === 5_000)![0] as () => void;
    await act(async () => refresh());
    await waitFor(() => expect(listCalls).toBe(2));
    await act(async () => refresh());
    await waitFor(() => expect(listCalls).toBe(3));
    expect(historyCalls).toBe(1);
    expect(await screen.findByTestId("performance-chart")).toHaveAttribute("data-point-count", "1");

    oldHistory.resolve(new Response(JSON.stringify([{ at: "2026-07-15T12:00:00Z", run_id: "run-old", cpu_percent: 99, memory_percent: null, network_rx_bytes_per_sec: null, network_tx_bytes_per_sec: null, block_read_bytes_per_sec: null, block_write_bytes_per_sec: null }]), { status: 200, headers: { "Content-Type": "application/json" } }));
    await waitFor(() => expect(screen.getByTestId("performance-chart")).toHaveAttribute("data-point-count", "3"));
    stalledDeletion.resolve(new Response("[]", { status: 200, headers: { "Content-Type": "application/json" } }));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(screen.getByText("深夜战役")).toBeInTheDocument();
    expect(screen.getByTestId("performance-chart")).toHaveAttribute("data-point-count", "3");

    await act(async () => refresh());
    await waitFor(() => expect(listCalls).toBe(4));
    expect(historyCalls).toBe(1);
  });

  it("does not continue session loading after unmount", async () => {
    const session = deferred<Response>();
    const calls: string[] = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      calls.push(String(input));
      if (String(input) === "/api/session") return session.promise;
      return new Response("[]", { status: 200, headers: { "Content-Type": "application/json" } });
    }));
    const view = render(<App />);
    expect(calls).toEqual(["/api/session"]);
    view.unmount();
    session.resolve(new Response('{"authenticated":true}', { status: 200, headers: { "Content-Type": "application/json" } }));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(calls).toEqual(["/api/session"]);
  });

  it("ignores the replayed StrictMode session effect and runs one current load path", async () => {
    const firstSession = deferred<Response>();
    const secondSession = deferred<Response>();
    const calls: string[] = [];
    let sessionCalls = 0;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      calls.push(path);
      if (path === "/api/session") {
        sessionCalls += 1;
        return sessionCalls === 1 ? firstSession.promise : secondSession.promise;
      }
      const value = path === "/api/instances" ? [apiInstance]
        : path === "/api/instances/1/overview" ? { ...runningZeroOverview, sampled_at: "2026-07-15T12:00:05Z", run_id: "run-1", container_running_known: true }
        : path === "/api/instances/1/performance-history" ? []
        : path === "/api/packages" ? [] : { ok: true };
      return new Response(JSON.stringify(value), { status: 200, headers: { "Content-Type": "application/json" } });
    }));

    render(<StrictMode><App /></StrictMode>);
    await waitFor(() => expect(sessionCalls).toBe(2));
    firstSession.resolve(new Response('{"authenticated":true}', { status: 200, headers: { "Content-Type": "application/json" } }));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(calls).toEqual(["/api/session", "/api/session"]);

    secondSession.resolve(new Response('{"authenticated":true}', { status: 200, headers: { "Content-Type": "application/json" } }));
    await waitFor(() => expect(calls.filter((path) => path === "/api/instances")).toHaveLength(1));
    await waitFor(() => expect(calls.filter((path) => path.endsWith("/performance-history"))).toHaveLength(1));
    expect(calls.filter((path) => path === "/api/packages")).toHaveLength(1);
    expect(calls.filter((path) => path === "/api/health")).toHaveLength(1);
    expect(calls.filter((path) => path.endsWith("/overview"))).toHaveLength(1);
  });

  it("keeps overview samples when the initial history request fails", async () => {
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      if (path.endsWith("/performance-history")) {
        return new Response('{"error":{"message":"history unavailable"}}', { status: 503, headers: { "Content-Type": "application/json" } });
      }
      const value = path === "/api/session" ? { authenticated: true }
        : path === "/api/instances" ? [apiInstance]
        : path.endsWith("/overview") ? { ...runningZeroOverview, sampled_at: "2026-07-15T12:00:05Z", run_id: "run-1", container_running_known: true, memory_limit_bytes: null, memory_percent: null, network_rx_bytes_per_sec: null, network_tx_bytes_per_sec: null, network_rx_bytes: null, network_tx_bytes: null, block_read_bytes_per_sec: null, block_write_bytes_per_sec: null, block_read_bytes: null, block_write_bytes: null, pids: null, uptime_seconds: null, a2s_latency_ms: null }
        : path === "/api/packages" ? [] : { ok: true };
      return new Response(JSON.stringify(value), { status: 200, headers: { "Content-Type": "application/json" } });
    }));
    render(<App />);
    expect(await screen.findByTestId("performance-chart")).toHaveAttribute("data-point-count", "1");
  });
  it("retries a failed history bootstrap on the next poll without clearing live samples", async () => {
    const intervalSpy = vi.spyOn(window, "setInterval");
    const failedHistory = deferred<Response>();
    let historyCalls = 0;
    let overviewCalls = 0;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      if (path === "/api/session") return new Response('{"authenticated":true}', { status: 200, headers: { "Content-Type": "application/json" } });
      if (path === "/api/instances") return new Response(JSON.stringify([apiInstance]), { status: 200, headers: { "Content-Type": "application/json" } });
      if (path.endsWith("/performance-history")) {
        historyCalls += 1;
        if (historyCalls === 1) return failedHistory.promise;
        return new Response("[]", { status: 200, headers: { "Content-Type": "application/json" } });
      }
      if (path.endsWith("/overview")) {
        overviewCalls += 1;
        return new Response(JSON.stringify({ ...runningZeroOverview, sampled_at: `2026-07-15T12:00:${overviewCalls === 1 ? "05" : "10"}Z`, run_id: "run-1", container_running_known: true }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      const value = path === "/api/packages" ? [] : { ok: true };
      return new Response(JSON.stringify(value), { status: 200, headers: { "Content-Type": "application/json" } });
    }));

    render(<App />);
    expect(await screen.findByTestId("performance-chart")).toHaveAttribute("data-point-count", "1");
    failedHistory.resolve(new Response('{"error":{"message":"unavailable"}}', { status: 503, headers: { "Content-Type": "application/json" } }));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    await waitFor(() => expect(intervalSpy.mock.calls.some(([, timeout]) => timeout === 5_000)).toBe(true));
    const refresh = intervalSpy.mock.calls.find(([, timeout]) => timeout === 5_000)![0] as () => void;
    await act(async () => refresh());
    await waitFor(() => expect(historyCalls).toBe(2));
    expect(await screen.findByTestId("performance-chart")).toHaveAttribute("data-point-count", "2");
  });

  it("removes the performance surface when an instance is deleted", async () => {
    const intervalSpy = vi.spyOn(window, "setInterval");
    let listCall = 0;
    let overviewCall = 0;
    let historyCalls = 0;
    const initialHistory = deferred<Response>();
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL) => {
      const path = String(input);
      let value: unknown;
      if (path === "/api/session") value = { authenticated: true };
      else if (path === "/api/instances") value = listCall++ === 1 ? [] : [apiInstance];
      else if (path.endsWith("/performance-history")) {
        historyCalls += 1;
        if (historyCalls === 1) return initialHistory.promise;
        value = [];
      } else if (path.endsWith("/overview")) {
        overviewCall += 1;
        value = { ...runningZeroOverview, sampled_at: overviewCall === 1 ? "2026-07-15T12:00:05Z" : "2026-07-15T12:00:20Z", run_id: "run-1", container_running_known: true, memory_limit_bytes: null, memory_percent: null, network_rx_bytes_per_sec: null, network_tx_bytes_per_sec: null, network_rx_bytes: null, network_tx_bytes: null, block_read_bytes_per_sec: null, block_write_bytes_per_sec: null, block_read_bytes: null, block_write_bytes: null, pids: null, uptime_seconds: null, a2s_latency_ms: null };
      } else value = path === "/api/packages" ? [] : { ok: true };
      return new Response(JSON.stringify(value), { status: 200, headers: { "Content-Type": "application/json" } });
    }));
    render(<App />);
    await waitFor(() => expect(intervalSpy.mock.calls.some(([, timeout]) => timeout === 5_000)).toBe(true));
    const refresh = intervalSpy.mock.calls.find(([, timeout]) => timeout === 5_000)![0] as () => void;
    initialHistory.resolve(new Response(JSON.stringify([{ at: "2026-07-15T12:00:00Z", run_id: "run-1", cpu_percent: 1, memory_percent: null, network_rx_bytes_per_sec: null, network_tx_bytes_per_sec: null, block_read_bytes_per_sec: null, block_write_bytes_per_sec: null }]), { status: 200, headers: { "Content-Type": "application/json" } }));
    expect(await screen.findByTestId("performance-chart")).toHaveAttribute("data-point-count", "2");
    await act(async () => refresh());
    await waitFor(() => expect(screen.queryByTestId("performance-chart")).not.toBeInTheDocument());
    expect(historyCalls).toBe(1);
  });
  it("shows operational instance data", () => {
    render(
      <App
        initialInstances={[instance]}
        initialPackages={[
          {
            id: "package-a",
            filename: "coop-a.zip",
            version: "v1",
            size: 1024,
            hot_compatible: true,
          },
        ]}
      />,
    );
    expect(screen.getByText("深夜战役")).toBeInTheDocument();
    expect(screen.getByText("4 / 8")).toBeInTheDocument();
    expect(screen.getByText("c2m1_highway")).toBeInTheDocument();
    expect(screen.getByText(/TV 27020.*插件 27021/)).toBeInTheDocument();
    expect(screen.getByText(/coop-a\.zip.*v1/)).toBeInTheDocument();
  });
  it("uses the live overview state instead of persisted running state", async () => {
    const calls: string[] = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const path = String(input);
        calls.push(path);
        const value =
          path === "/api/session"
            ? { authenticated: true }
            : path === "/api/instances"
              ? [apiInstance]
              : path === "/api/instances/1/overview"
                ? stoppedOverview
                : path === "/api/packages"
                  ? []
                  : path === "/api/health"
                    ? { ok: true }
                    : { error: { message: "unexpected request" } };
        return new Response(JSON.stringify(value), {
          status: "error" in value ? 404 : 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );

    render(<App />);

    const card = (await screen.findByText("深夜战役")).closest("article");
    expect(card).not.toBeNull();
    expect(await within(card!).findByText("已停止")).toBeInTheDocument();
    expect(within(card!).getByText("-- / --")).toBeInTheDocument();
    expect(within(card!).getAllByText("--").length).toBeGreaterThanOrEqual(2);
    expect(card).toHaveClass("stopped");
    const playerTotal = screen.getByText("在线玩家").closest("article");
    expect(playerTotal).not.toBeNull();
    expect(within(playerTotal!).getByText("--")).toBeInTheDocument();
    expect(screen.getByText("实时观测状态")).toBeInTheDocument();
    expect(calls).toContain("/api/instances/1/overview");
    expect(calls).not.toContain("/api/instances/1/players");
    expect(calls).not.toContain("/api/instances/1/resources");
  });
  it("polls overview observations and preserves legitimate zero metrics", async () => {
    const intervalSpy = vi.spyOn(window, "setInterval");
    const observations = [stoppedOverview, runningZeroOverview];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const path = String(input);
        const value =
          path === "/api/session"
            ? { authenticated: true }
            : path === "/api/instances"
              ? [apiInstance]
              : path === "/api/instances/1/overview"
                ? observations.shift() || runningZeroOverview
                : path === "/api/packages"
                  ? []
                  : { ok: true };
        return new Response(JSON.stringify(value), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );

    render(<App />);
    expect(await screen.findByText("-- / --")).toBeInTheDocument();
    await waitFor(() => expect(intervalSpy.mock.calls.some(([, timeout]) => timeout === 5_000)).toBe(true));
    const refresh = intervalSpy.mock.calls.find(([, timeout]) => timeout === 5_000)![0] as () => void;

    await act(async () => {
      refresh();
    });

    expect(await screen.findByText("0 / 8")).toBeInTheDocument();
    expect(screen.getByText("0%")).toBeInTheDocument();
    expect(screen.getByText("0 B / -- (--)")).toBeInTheDocument();
    expect(screen.getByText("运行中")).toBeInTheDocument();
  });
  it("opens the existing instance configuration from its card", async () => {
    render(
      <App
        initialInstances={[instance]}
        initialPackages={[
          {
            id: "package-a",
            filename: "coop-a.zip",
            version: "v1",
            size: 1024,
            hot_compatible: true,
          },
        ]}
      />,
    );
    await userEvent.click(
      screen.getByRole("button", { name: "配置 深夜战役" }),
    );
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByLabelText("额外 SRCDS 启动项")).toHaveValue(
      "-strictportbind",
    );
  });
  it("requires confirmation before stopping", async () => {
    const onAction = vi.fn();
    render(<App initialInstances={[instance]} onAction={onAction} />);
    await userEvent.click(
      screen.getByRole("button", { name: "停止 深夜战役" }),
    );
    expect(onAction).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "确认停止" }));
    expect(onAction).toHaveBeenCalledWith("1", "stop");
  });
  it("submits SourceTV and plugin ports when creating an instance", async () => {
    let submitted: Record<string, unknown> | undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => {
        if (init?.method === "POST") {
          submitted = JSON.parse(String(init.body));
        }
        return new Response(init?.method === "POST" ? "{}" : "[]", {
          status: 201,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(
      <App
        initialInstances={[]}
        initialPackages={[
          {
            id: "package-a",
            filename: "coop-a.zip",
            version: "v1",
            size: 1024,
            hot_compatible: true,
          },
        ]}
      />,
    );
    await userEvent.click(screen.getByRole("button", { name: "创建实例" }));
    await userEvent.type(screen.getByLabelText("名称"), "端口测试");
    await userEvent.clear(screen.getByLabelText("SourceTV 端口"));
    await userEvent.type(screen.getByLabelText("SourceTV 端口"), "27020");
    await userEvent.type(screen.getByLabelText("插件端口"), "27021, 27022");
    await userEvent.click(screen.getByRole("button", { name: "创建" }));
    expect(submitted).toMatchObject({
      sourcetv_port: 27020,
      plugin_ports: [27021, 27022],
      package_id: "package-a",
    });
  });
  it("logs in and loads real instances", async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(
        new Response("{}", {
          status: 401,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response('{"authenticated":true}', {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      )
      .mockResolvedValueOnce(
        new Response("[]", {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      );
    vi.stubGlobal("fetch", fetchMock);
    render(<App />);
    expect(await screen.findByText("管理员认证")).toBeInTheDocument();
    await userEvent.type(
      screen.getByLabelText("管理员密码"),
      "correct horse battery staple",
    );
    await userEvent.click(screen.getByRole("button", { name: "进入作战室" }));
    expect(
      await screen.findByText("尚无实例。创建第一个 Host 网络服务器。"),
    ).toBeInTheDocument();
    vi.unstubAllGlobals();
  });
  it("opens private files as an independent main navigation page", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const path = String(input);
        const body = path.endsWith("/private/diff")
          ? '{"changes":[],"summary":{"added":0,"modified":0,"deleted":0}}'
          : "[]";
        return new Response(body, {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "私有文件" }));
    expect(await screen.findByRole("heading", { name: "私有文件" })).toBeVisible();
    vi.unstubAllGlobals();
  });
  it("disables instance-scoped content actions when no instance exists", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const path = String(input);
        const body =
          path === "/api/packages"
            ? '[{"id":"pkg-1","filename":"plugins.zip","version":"v1","size":4,"hot_compatible":true}]'
            : "[]";
        return new Response(body, {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[]} />);
    await userEvent.click(screen.getByRole("button", { name: "内容仓库" }));
    expect(
      await screen.findByRole("button", { name: "热更新" }),
    ).toBeDisabled();
    expect(screen.getByRole("button", { name: "完整更新" })).toBeDisabled();
    await waitFor(() =>
      expect(screen.getByText("plugins.zip · v1")).toBeInTheDocument(),
    );
    vi.unstubAllGlobals();
  });
  it("shows persisted jobs on a dedicated operations page", async () => {
    const fetchMock = vi.fn(async (input: RequestInfo | URL) => {
      if (String(input) === "/api/jobs") {
        return new Response(
          JSON.stringify([
            {
              ID: "job-1",
              Type: "game_update",
              Status: "failed",
              Stage: "steamcmd",
              Percent: 37,
              Error: "download interrupted",
              CreatedAt: "2026-07-16T08:00:00Z",
              UpdatedAt: "2026-07-16T08:02:20Z",
              StartedAt: "2026-07-16T08:00:02Z",
              FinishedAt: "2026-07-16T08:02:20Z",
            },
          ]),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      if (String(input) === "/api/jobs/job-1") {
        return new Response(
          JSON.stringify({
            ID: "job-1",
            Type: "game_update",
            Status: "failed",
            Stage: "steamcmd",
            Percent: 37,
            Error: "download interrupted",
            CreatedAt: "2026-07-16T08:00:00Z",
            UpdatedAt: "2026-07-16T08:02:20Z",
            StartedAt: "2026-07-16T08:00:02Z",
            FinishedAt: "2026-07-16T08:02:20Z",
            Events: [
              {
                ID: 1,
                JobID: "job-1",
                Kind: "failed",
                Stage: "steamcmd",
                Percent: 37,
                Message: "download interrupted",
                CreatedAt: "2026-07-16T08:02:20Z",
              },
            ],
          }),
          { status: 200, headers: { "Content-Type": "application/json" } },
        );
      }
      return new Response("[]", {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    });
    vi.stubGlobal(
      "fetch",
      fetchMock,
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "任务" }));
    expect(await screen.findByText("game_update")).toBeInTheDocument();
    expect(screen.getByText("download interrupted")).toBeInTheDocument();
    await userEvent.click(
      screen.getByRole("button", {
        name: "查看 game_update 任务日志，任务 ID job-1",
      }),
    );
    expect(
      await screen.findByRole("region", { name: "game_update 任务日志" }),
    ).toHaveTextContent("download interrupted");
    expect(screen.getByText("执行用时 2分18秒")).toBeVisible();
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/jobs/job-1",
      expect.objectContaining({ credentials: "same-origin" }),
    );
    vi.unstubAllGlobals();
  });
  it("loads VPK downloads from the content repository", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const path = String(input);
        if (path === "/api/content/vpk") {
          return new Response(
            '[{"name":"maps.vpk","size":1024,"hash":"abcdef"}]',
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        if (path === "/api/packages") {
          return new Response("[]", {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        return new Response("[]", {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "内容仓库" }));
    const download = await screen.findByRole("link", { name: "下载 maps.vpk" });
    expect(download).toHaveAttribute(
      "href",
      "/api/content/vpk/maps.vpk/download",
    );
    vi.unstubAllGlobals();
  });

  it("reports the real control-node health", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL) => {
        const path = String(input);
        if (path === "/api/session") {
          return new Response('{"authenticated":true}', {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (path === "/api/instances") {
          return new Response("[]", {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        return new Response(
          JSON.stringify({ error: { message: "Docker proxy unavailable" } }),
          { status: 503, headers: { "Content-Type": "application/json" } },
        );
      }),
    );
    render(<App />);
    expect(await screen.findByText("控制节点异常")).toBeInTheDocument();
    expect(screen.getByText("Docker proxy unavailable")).toBeInTheDocument();
  });

  it("shows Cron save success and sends the snake-case contract", async () => {
    let saved = false;
    let submitted: Record<string, unknown> | undefined;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = String(input);
        if (path === "/api/schedules" && init?.method === "POST") {
          submitted = JSON.parse(String(init.body));
          saved = true;
          return new Response(JSON.stringify({ id: "schedule-1" }), {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        const body =
          path === "/api/schedules" && saved
            ? '[{"id":"schedule-1","type":"game_update","cron":"0 4 * * *","timezone":"Asia/Hong_Kong","enabled":true}]'
            : "[]";
        return new Response(body, {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "计划任务" }));
    await userEvent.click(screen.getByRole("button", { name: "保存计划" }));
    expect(await screen.findByRole("status")).toHaveTextContent("计划已保存");
    expect(submitted).toMatchObject({
      instance_id: "1",
      online_policy: "skip",
    });
  });

  it("shows Cron save errors inline", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) =>
        init?.method === "POST"
          ? new Response(
              JSON.stringify({ error: { message: "invalid Cron expression" } }),
              { status: 422, headers: { "Content-Type": "application/json" } },
            )
          : new Response("[]", {
              status: 200,
              headers: { "Content-Type": "application/json" },
            }),
      ),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "计划任务" }));
    await userEvent.click(screen.getByRole("button", { name: "保存计划" }));
    expect(await screen.findByRole("alert")).toHaveTextContent(
      "invalid Cron expression",
    );
  });

  it("describes Steam credentials as optional for anonymous installs", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () =>
        new Response('{"configured":false}', {
          status: 200,
          headers: { "Content-Type": "application/json" },
        }),
      ),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "系统设置" }));
    expect(
      await screen.findByText("匿名首装已支持；仅许可账号需要配置凭据"),
    ).toBeInTheDocument();
  });

  it("loads and updates the successful job retention limit", async () => {
    const fetchMock = vi.fn(
      async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = String(input);
        if (path === "/api/settings/jobs") {
          if (init?.method === "PUT") {
            return Response.json({ successful_job_limit: 40 });
          }
          return Response.json({ successful_job_limit: 25 });
        }
        return Response.json({ configured: false });
      },
    );
    vi.stubGlobal("fetch", fetchMock);

    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "系统设置" }));
    const input = await screen.findByRole("spinbutton", {
      name: "成功任务保留数量",
    });
    expect(input).toHaveValue(25);
    expect(input).toHaveAttribute("min", "1");
    expect(input).toHaveAttribute("max", "500");
    expect(
      screen.getByText("仅限制成功任务；失败和中断任务不会自动删除。"),
    ).toBeVisible();

    await userEvent.clear(input);
    await userEvent.type(input, "40");
    await userEvent.click(
      screen.getByRole("button", { name: "保存任务记录设置" }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/settings/jobs",
      expect.objectContaining({
        method: "PUT",
        body: JSON.stringify({ successful_job_limit: 40 }),
      }),
    );
    expect(await screen.findByRole("status")).toHaveTextContent(
      "任务记录设置已保存",
    );
  });

  it("disables retention settings while the save is in progress", async () => {
    const save = deferred<Response>();
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        if (String(input) === "/api/settings/jobs") {
          if (init?.method === "PUT") return save.promise;
          return Response.json({ successful_job_limit: 25 });
        }
        return Response.json({ configured: false });
      }),
    );

    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "系统设置" }));
    const saveButton = await screen.findByRole("button", {
      name: "保存任务记录设置",
    });
    await waitFor(() => expect(saveButton).toBeEnabled());
    await userEvent.click(saveButton);
    expect(saveButton).toBeDisabled();

    save.resolve(Response.json({ successful_job_limit: 25 }));
    await waitFor(() => expect(saveButton).toBeEnabled());
  });

  it("restores the confirmed retention limit when saving fails", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        if (String(input) === "/api/settings/jobs") {
          if (init?.method === "PUT") {
            return Response.json(
              { error: { message: "保存任务设置失败" } },
              { status: 500 },
            );
          }
          return Response.json({ successful_job_limit: 25 });
        }
        return Response.json({ configured: false });
      }),
    );

    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "系统设置" }));
    const input = await screen.findByRole("spinbutton", {
      name: "成功任务保留数量",
    });
    await userEvent.clear(input);
    await userEvent.type(input, "40");
    await userEvent.click(
      screen.getByRole("button", { name: "保存任务记录设置" }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "保存任务设置失败",
    );
    expect(input).toHaveValue(25);
  });

  it("selects both forced reinstall targets by default", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response('{"ID":"job-1","Status":"pending"}', {
        status: 202,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "更新" }));
    expect(fetchMock).not.toHaveBeenCalled();
    expect(screen.getByRole("dialog")).toHaveTextContent("重新安装实例组件");
    expect(screen.getByRole("checkbox", { name: "重新安装游戏本体" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "重新安装实例插件包" })).toBeChecked();
    await userEvent.click(
      screen.getByRole("button", { name: "确认重新安装" }),
    );
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/instances/1/game-update",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({
          confirm: true,
          reinstall_game: true,
          reinstall_package: true,
        }),
      }),
    );
  });

  it("submits one selected reinstall target and disables an empty selection", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response('{"ID":"job-1","Status":"pending"}', {
        status: 202,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "更新" }));
    const game = screen.getByRole("checkbox", { name: "重新安装游戏本体" });
    const packageOption = screen.getByRole("checkbox", { name: "重新安装实例插件包" });
    await userEvent.click(packageOption);
    await userEvent.click(screen.getByRole("button", { name: "确认重新安装" }));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/instances/1/game-update",
      expect.objectContaining({
        body: JSON.stringify({
          confirm: true,
          reinstall_game: true,
          reinstall_package: false,
        }),
      }),
    );

    await userEvent.click(screen.getByRole("button", { name: "更新" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "重新安装游戏本体" }));
    await userEvent.click(screen.getByRole("checkbox", { name: "重新安装实例插件包" }));
    expect(screen.getByRole("button", { name: "确认重新安装" })).toBeDisabled();
  });

  it("does not request a package reinstall when the instance has no selected package", async () => {
    const fetchMock = vi.fn().mockResolvedValue(
      new Response('{"ID":"job-1","Status":"pending"}', {
        status: 202,
        headers: { "Content-Type": "application/json" },
      }),
    );
    vi.stubGlobal("fetch", fetchMock);
    render(<App initialInstances={[{ ...instance, package_id: "", applied_package_id: "" }]} />);
    await userEvent.click(screen.getByRole("button", { name: "更新" }));
    expect(screen.getByRole("checkbox", { name: "重新安装游戏本体" })).toBeChecked();
    expect(screen.getByRole("checkbox", { name: "重新安装实例插件包" })).not.toBeChecked();
    expect(screen.getByRole("checkbox", { name: "重新安装实例插件包" })).toBeDisabled();
    await userEvent.click(screen.getByRole("button", { name: "确认重新安装" }));
    expect(fetchMock).toHaveBeenCalledWith(
      "/api/instances/1/game-update",
      expect.objectContaining({
        body: JSON.stringify({
          confirm: true,
          reinstall_game: true,
          reinstall_package: false,
        }),
      }),
    );
  });

  it("confirms full package updates before submitting them", async () => {
    const calls: Array<[RequestInfo | URL, RequestInit | undefined]> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        calls.push([input, init]);
        const path = String(input);
        const body = path === "/api/packages"
          ? '[{"id":"pkg-1","filename":"plugins.zip","version":"v1","size":4,"hot_compatible":true}]'
          : "[]";
        return new Response(body, {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "内容仓库" }));
    await userEvent.click(
      await screen.findByRole("button", { name: "完整更新" }),
    );
    expect(calls.some(([, init]) => init?.method === "POST")).toBe(false);
    await userEvent.click(
      screen.getByRole("button", { name: "确认完整更新" }),
    );
    expect(
      calls.some(
        ([path, init]) =>
          String(path) === "/api/instances/1/updates" &&
          init?.method === "POST" &&
          String(init.body).includes('"confirm":true'),
      ),
    ).toBe(true);
  });

  it("checks the configured GitHub Release without applying it", async () => {
    const calls: Array<[RequestInfo | URL, RequestInit | undefined]> = [];
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      calls.push([input, init]);
      const path = String(input);
      const response = path === "/api/github-sources" ? '[{"id":"default","name":"默认源","repository":"owner/repo","asset_pattern":"^plugins[.]zip$"}]' : path.endsWith("/check") ? '{"ID":"job-1","Status":"pending"}' : "[]";
      return new Response(response, {
        status: path.endsWith("/check") ? 202 : 200,
        headers: { "Content-Type": "application/json" },
      });
    }));
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "内容仓库" }));
    await userEvent.click(await screen.findByRole("button", { name: "检查更新 默认源" }));
    expect(calls).toContainEqual([
      "/api/github-sources/default/check",
      expect.objectContaining({
        method: "POST",
        body: JSON.stringify({}),
      }),
    ]);
  });

  it("saves independent scheduled Release update modes", async () => {
    let submitted: any;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      const path = String(input);
      if (path === "/api/schedules" && init?.method === "POST") submitted = JSON.parse(String(init.body));
      const response = path === "/api/github-sources" ? '[{"id":"source-1","name":"源一","repository":"owner/repo","asset_pattern":"^plugins[.]zip$"}]' : init?.method === "POST" ? "{}" : "[]";
      return new Response(response, { status: 200, headers: { "Content-Type": "application/json" } });
    }));
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "计划任务" }));
    await userEvent.selectOptions(screen.getByLabelText("任务"), "release_hot");
    expect(screen.getByLabelText("GitHub 源")).toBeInTheDocument();
    await userEvent.click(screen.getByRole("button", { name: "保存计划" }));
    expect(submitted.type).toBe("release_hot");
    expect(JSON.parse(submitted.payload)).toEqual({ source_id: "source-1" });
  });

  it("confirms player kicks and bans before submitting them", async () => {
    const calls: Array<[RequestInfo | URL, RequestInit | undefined]> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        calls.push([input, init]);
        if (String(input).endsWith("/players") && !init?.method) {
          return new Response(
            '{"map":"c2m1_highway","max_players":12,"match":{"hostname":"6","version":"2.2.4.3 10097","secure":true,"os":"Linux Dedicated","map":"c2m1_highway","private_address":"127.0.1.1:27991","public_address":"221.215.78.153:27991","humans":1,"max_players":12},"players":[{"name":"Ellis","user_id":7,"unique_id":"STEAM_1:0:42","connected":"00:48","ping":29,"loss":0,"score":2}]}',
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response('{"ID":"job-1","Status":"pending"}', {
          status: 202,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "玩家" }));
    const playerDialog = await screen.findByRole("dialog", { name: "深夜战役" });
    expect(within(playerDialog).getByText("c2m1_highway")).toBeVisible();
    expect(within(playerDialog).getByText("1 / 12")).toBeVisible();
    expect(within(playerDialog).getByText("2.2.4.3 10097 · 安全")).toBeVisible();
    expect(within(playerDialog).getByText("STEAM_1:0:42")).toBeVisible();
    expect(within(playerDialog).getByText("00:48")).toBeVisible();
    expect(within(playerDialog).getByText("29 ms")).toBeVisible();
    expect(within(playerDialog).getByText("0%")).toBeVisible();
    await userEvent.click(await screen.findByRole("button", { name: "踢出" }));
    expect(calls.some(([, init]) => init?.method === "POST")).toBe(false);
    await userEvent.click(screen.getByRole("button", { name: "确认踢出" }));
    expect(calls.some(([, init]) => init?.method === "POST")).toBe(true);

    await userEvent.click(screen.getByRole("button", { name: "永久封禁" }));
    expect(screen.getByRole("dialog", { name: "永久封禁 Ellis？" })).toHaveTextContent("永久封禁 Ellis");
    await userEvent.click(screen.getByRole("button", { name: "确认永久封禁" }));
    expect(
      calls.some(([, init]) =>
        String(init?.body).includes('"action":"ban"'),
      ),
    ).toBe(true);
  });

  it("treats interrupted jobs as terminal and keeps their error visible", async () => {
    vi.useFakeTimers();
    let jobReads = 0;
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = String(input);
        if (init?.method === "POST") {
          return new Response('{"ID":"job-1","Status":"pending"}', {
            status: 202,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (path === "/api/jobs/job-1") {
          jobReads += 1;
          return new Response(
            '{"ID":"job-1","Status":"interrupted","Error":"Panel restarted; inspect and retry"}',
            { status: 200, headers: { "Content-Type": "application/json" } },
          );
        }
        return new Response("[]", {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[{ ...instance, actual_state: "stopped" }]} />);
    fireEvent.click(screen.getByRole("button", { name: "启动" }));
    await act(async () => {
      await Promise.resolve();
      vi.advanceTimersByTime(1_700);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(jobReads).toBe(1);
    expect(screen.getByText("Panel restarted; inspect and retry")).toBeInTheDocument();
  });

  it("serializes slow job polling and stops polling after unmount", async () => {
    vi.useFakeTimers();
    let jobReads = 0;
    let resolveRead!: (response: Response) => void;
    vi.stubGlobal("fetch", vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      if (init?.method === "POST") return new Response('{"ID":"slow-job","Status":"pending"}', { status: 202, headers: { "Content-Type": "application/json" } });
      if (String(input) === "/api/jobs/slow-job") {
        jobReads++;
        return new Promise<Response>((resolve) => { resolveRead = resolve; });
      }
      return new Response("[]", { status: 200, headers: { "Content-Type": "application/json" } });
    }));
    const view = render(<App initialInstances={[{ ...instance, actual_state: "stopped" }]} />);
    fireEvent.click(screen.getByRole("button", { name: "启动" }));
    await act(async () => { await Promise.resolve(); await Promise.resolve(); });
    expect(jobReads).toBe(1);
    await act(async () => { vi.advanceTimersByTime(5_000); await Promise.resolve(); });
    expect(jobReads).toBe(1);
    view.unmount();
    resolveRead(new Response('{"ID":"slow-job","Status":"running"}', { status: 200, headers: { "Content-Type": "application/json" } }));
    await act(async () => { await Promise.resolve(); vi.advanceTimersByTime(5_000); });
    expect(jobReads).toBe(1);
  });

  it("hashes and uploads VPK files in sequential 8 MiB chunks", async () => {
    const chunkSize = 8 * 1024 * 1024;
    const patchCalls: Array<{ offset: number; size: number }> = [];
    vi.stubGlobal(
      "fetch",
      vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
        const path = String(input);
        if (path === "/api/content/vpk/uploads" && init?.method === "POST") {
          return new Response('{"id":"upload-1"}', {
            status: 201,
            headers: { "Content-Type": "application/json" },
          });
        }
        if (init?.method === "PATCH") {
          patchCalls.push({
            offset: Number(new URL(path, "http://panel.test").searchParams.get("offset")),
            size: (init.body as Blob).size,
          });
          return new Response("{}", {
            status: 200,
            headers: { "Content-Type": "application/json" },
          });
        }
        return new Response(path.includes("/complete") ? "{}" : "[]", {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      }),
    );
    render(<App initialInstances={[instance]} />);
    await userEvent.click(screen.getByRole("button", { name: "内容仓库" }));
    const wholeRead = vi.fn(() => Promise.reject(new Error("whole-file read")));
    const fakeFile = {
      name: "maps.vpk",
      size: chunkSize + 3,
      arrayBuffer: wholeRead,
      slice: vi.fn((start: number, end: number) => {
        const size = end - start;
        return {
          size,
          arrayBuffer: async () => new Uint8Array(size).buffer,
        } as Blob;
      }),
    } as unknown as File;
    fireEvent.change(screen.getByLabelText("上传 VPK"), {
      target: { files: [fakeFile] },
    });
    await waitFor(() => expect(patchCalls).toHaveLength(2));
    expect(patchCalls).toEqual([
      { offset: 0, size: chunkSize },
      { offset: chunkSize, size: 3 },
    ]);
    expect(wholeRead).not.toHaveBeenCalled();
    expect(screen.getByRole("status")).toHaveTextContent("VPK 上传完成");
  });
});

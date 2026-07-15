import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { useConsoleFollow } from "./useConsoleFollow";

type Geometry = {
  scrollHeight: number;
  clientHeight: number;
  scrollTop: number;
};

function installRaf() {
  let nextID = 1;
  const callbacks = new Map<number, FrameRequestCallback>();
  vi.stubGlobal("requestAnimationFrame", vi.fn((callback: FrameRequestCallback) => {
    const id = nextID++;
    callbacks.set(id, callback);
    return id;
  }));
  vi.stubGlobal("cancelAnimationFrame", vi.fn((id: number) => callbacks.delete(id)));
  return {
    flush() {
      const pending = [...callbacks.entries()];
      callbacks.clear();
      pending.forEach(([id, callback]) => callback(id));
    },
  };
}

function attach(
  ref: { current: HTMLPreElement | null },
  geometry: Geometry,
) {
  const element = document.createElement("pre");
  Object.defineProperties(element, {
    scrollHeight: { configurable: true, get: () => geometry.scrollHeight },
    clientHeight: { configurable: true, get: () => geometry.clientHeight },
    scrollTop: {
      configurable: true,
      get: () => geometry.scrollTop,
      set: (value: number) => { geometry.scrollTop = value; },
    },
  });
  ref.current = element;
  return element;
}

afterEach(() => vi.unstubAllGlobals());

describe("useConsoleFollow", () => {
  it("forces the console to the bottom when opened", () => {
    const raf = installRaf();
    const geometry = { scrollHeight: 300, clientHeight: 100, scrollTop: 0 };
    const { result } = renderHook(() => useConsoleFollow(0));
    attach(result.current.outputRef, geometry);

    act(() => result.current.forceFollow());
    act(() => raf.flush());

    expect(geometry.scrollTop).toBe(300);
    expect(result.current.isFollowing()).toBe(true);
  });

  it("forces follow before a command is submitted", () => {
    const raf = installRaf();
    const geometry = { scrollHeight: 400, clientHeight: 100, scrollTop: 20 };
    const { result } = renderHook(() => useConsoleFollow(0));
    const element = attach(result.current.outputRef, geometry);
    act(() => raf.flush());
    geometry.scrollTop = 20;
    act(() => result.current.onScroll({ currentTarget: element } as never));
    expect(result.current.isFollowing()).toBe(false);

    act(() => result.current.forceFollow());
    act(() => raf.flush());

    expect(geometry.scrollTop).toBe(400);
    expect(result.current.isFollowing()).toBe(true);
  });

  it("scrolls after appended output only while following", () => {
    const raf = installRaf();
    const geometry = { scrollHeight: 300, clientHeight: 100, scrollTop: 200 };
    const { result, rerender } = renderHook(
      ({ version }) => useConsoleFollow(version),
      { initialProps: { version: 0 } },
    );
    const element = attach(result.current.outputRef, geometry);
    act(() => result.current.forceFollow());
    act(() => raf.flush());

    geometry.scrollHeight = 360;
    rerender({ version: 1 });
    act(() => raf.flush());
    expect(geometry.scrollTop).toBe(360);

    geometry.scrollTop = 100;
    act(() => result.current.onScroll({ currentTarget: element } as never));
    geometry.scrollHeight = 420;
    rerender({ version: 2 });
    act(() => raf.flush());
    expect(geometry.scrollTop).toBe(100);
  });

  it("resumes following at the bottom and uses a four-pixel tolerance", () => {
    const raf = installRaf();
    const geometry = { scrollHeight: 300, clientHeight: 100, scrollTop: 195 };
    const { result } = renderHook(() => useConsoleFollow(0));
    const element = attach(result.current.outputRef, geometry);
    act(() => raf.flush());
    geometry.scrollTop = 195;

    act(() => result.current.onScroll({ currentTarget: element } as never));
    expect(result.current.isFollowing()).toBe(false);
    geometry.scrollTop = 196;
    act(() => result.current.onScroll({ currentTarget: element } as never));
    expect(result.current.isFollowing()).toBe(true);
    geometry.scrollTop = 200;
    act(() => result.current.onScroll({ currentTarget: element } as never));
    expect(result.current.isFollowing()).toBe(true);
  });

  it("preserves a manual pause across a mounted reconnect and resets on remount", () => {
    const raf = installRaf();
    const geometry = { scrollHeight: 300, clientHeight: 100, scrollTop: 50 };
    const { result, rerender, unmount } = renderHook(
      ({ version }) => useConsoleFollow(version),
      { initialProps: { version: 0 } },
    );
    const element = attach(result.current.outputRef, geometry);
    act(() => raf.flush());
    geometry.scrollTop = 50;
    act(() => result.current.onScroll({ currentTarget: element } as never));
    rerender({ version: 1 });
    act(() => raf.flush());
    expect(result.current.isFollowing()).toBe(false);
    expect(geometry.scrollTop).toBe(50);
    unmount();

    const remounted = renderHook(() => useConsoleFollow(0));
    attach(remounted.result.current.outputRef, geometry);
    act(() => remounted.result.current.forceFollow());
    act(() => raf.flush());
    expect(remounted.result.current.isFollowing()).toBe(true);
    expect(geometry.scrollTop).toBe(300);
  });

  it("lets a user scroll cancel a pending forced follow", () => {
    const raf = installRaf();
    const geometry = { scrollHeight: 300, clientHeight: 100, scrollTop: 50 };
    const { result } = renderHook(() => useConsoleFollow(0));
    const element = attach(result.current.outputRef, geometry);

    act(() => result.current.forceFollow());
    geometry.scrollTop = 50;
    act(() => result.current.onScroll({ currentTarget: element } as never));
    expect(result.current.isFollowing()).toBe(false);
    act(() => raf.flush());
    expect(geometry.scrollTop).toBe(50);
  });

  it("keeps a user pause after sustained output reschedules the frame", () => {
    const raf = installRaf();
    const geometry = { scrollHeight: 300, clientHeight: 100, scrollTop: 50 };
    const { result, rerender } = renderHook(
      ({ version }) => useConsoleFollow(version),
      { initialProps: { version: 0 } },
    );
    const element = attach(result.current.outputRef, geometry);

    rerender({ version: 1 });
    rerender({ version: 2 });
    geometry.scrollTop = 50;
    act(() => result.current.onScroll({ currentTarget: element } as never));
    expect(result.current.isFollowing()).toBe(false);
    act(() => raf.flush());
    expect(geometry.scrollTop).toBe(50);
  });

  it("cancels a pending animation frame on unmount", () => {
    const raf = installRaf();
    const geometry = { scrollHeight: 300, clientHeight: 100, scrollTop: 0 };
    const { result, unmount } = renderHook(() => useConsoleFollow(0));
    attach(result.current.outputRef, geometry);

    act(() => result.current.forceFollow());
    unmount();
    expect(cancelAnimationFrame).toHaveBeenCalled();
    act(() => raf.flush());
    expect(geometry.scrollTop).toBe(0);
  });
});

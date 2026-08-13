import { describe, expect, it } from "vitest";
import { formatStackwalkFrame, getCrashedThreadTopFrame, parseStackwalk } from "./stackwalk";

describe("parseStackwalk", () => {
  it("splits compact frames into module, symbol and offset", () => {
    expect(parseStackwalk("#0 server!Crash+0x10\n#1 engine!Run+0x20")).toEqual([
      {
        id: "0",
        crashed: false,
        frames: [
          { kind: "frame", index: "0", module: "server", symbol: "Crash", offset: "0x10", raw: "#0 server!Crash+0x10" },
          { kind: "frame", index: "1", module: "engine", symbol: "Run", offset: "0x20", raw: "#1 engine!Run+0x20" },
        ],
      },
    ]);
  });

  it("groups frames by thread and drops stackwalk logs", () => {
    expect(parseStackwalk([
      "Thread 0 (crashed)",
      "0 libc.so.6 + 0x9929b eip = 0xf7da629b",
      "    Found by: given as instruction pointer in context",
      "3 libtier0_srv.so!Plat_FloatTime + 0x2b eip = 0xf7cca02b",
      "    Found by: stack scanning",
      "minidump.cc:5573: INFO: Minidump closing minidump",
      "Thread 1",
      "0 engine_srv.so!RunFrame + 0x20",
      "Loaded modules:",
    ].join("\n"))).toEqual([
      {
        id: "0",
        crashed: true,
        frames: [
          { kind: "frame", index: "0", module: "libc.so.6", offset: "0x9929b", foundBy: "given as instruction pointer in context", raw: "0 libc.so.6 + 0x9929b eip = 0xf7da629b\n    Found by: given as instruction pointer in context" },
          { kind: "frame", index: "3", module: "libtier0_srv.so", symbol: "Plat_FloatTime", offset: "0x2b", foundBy: "stack scanning", raw: "3 libtier0_srv.so!Plat_FloatTime + 0x2b eip = 0xf7cca02b\n    Found by: stack scanning" },
        ],
      },
      {
        id: "1",
        crashed: false,
        frames: [
          { kind: "frame", index: "0", module: "engine_srv.so", symbol: "RunFrame", offset: "0x20", raw: "0 engine_srv.so!RunFrame + 0x20" },
        ],
      },
    ]);
  });

  it("drops numeric preamble lines when the stackwalk has explicit thread headings", () => {
    expect(parseStackwalk([
      "Operating system: Linux",
      "CPU: x86",
      "     1 CPU",
      "Thread 0 (crashed)",
      " 0  engine_srv.so + 0x16bea2",
      "    Found by: given as instruction pointer in context",
      " 1  metamod.2.l4d2.so + 0x2d480",
      "    Found by: previous frame's frame pointer",
      "Thread 1",
      " 0  linux-gate.so + 0x579",
    ].join("\n"))).toEqual([
      {
        id: "0",
        crashed: true,
        frames: [
          { kind: "frame", index: "0", module: "engine_srv.so", offset: "0x16bea2", foundBy: "given as instruction pointer in context", raw: " 0  engine_srv.so + 0x16bea2\n    Found by: given as instruction pointer in context" },
          { kind: "frame", index: "1", module: "metamod.2.l4d2.so", offset: "0x2d480", foundBy: "previous frame's frame pointer", raw: " 1  metamod.2.l4d2.so + 0x2d480\n    Found by: previous frame's frame pointer" },
        ],
      },
      {
        id: "1",
        crashed: false,
        frames: [
          { kind: "frame", index: "0", module: "linux-gate.so", offset: "0x579", raw: " 0  linux-gate.so + 0x579" },
        ],
      },
    ]);
  });

  it("parses Breakpad source locations and keeps Found by after register continuations", () => {
    const [thread] = parseStackwalk([
      "Thread 0 (crashed)",
      " 0 libc.so.6 + 0x9929b",
      "    eip = 0xf7da629b   esp = 0xee3982d0",
      "    Found by: given as instruction pointer in context",
      " 2 socket.ext.so!Scheduler::Run [scheduler.ipp : 431 + 0x10]",
      "    eip = 0xde0ffd44",
      "    Found by: call frame info",
    ].join("\n"));

    expect(thread.frames).toEqual([
      {
        kind: "frame",
        index: "0",
        module: "libc.so.6",
        offset: "0x9929b",
        foundBy: "given as instruction pointer in context",
        raw: " 0 libc.so.6 + 0x9929b\n    Found by: given as instruction pointer in context",
      },
      {
        kind: "frame",
        index: "2",
        module: "socket.ext.so",
        symbol: "Scheduler::Run",
        offset: "0x10",
        foundBy: "call frame info",
        raw: " 2 socket.ext.so!Scheduler::Run [scheduler.ipp : 431 + 0x10]\n    Found by: call frame info",
      },
    ]);
  });

  it("extracts a source annotation appended to the frame line", () => {
    expect(parseStackwalk("0 libc.so.6 + 0x9929b eip = 0xf7da629b Found by: given as instruction pointer in context")).toEqual([
      {
        id: "0",
        crashed: false,
        frames: [
          {
            kind: "frame",
            index: "0",
            module: "libc.so.6",
            offset: "0x9929b",
            foundBy: "given as instruction pointer in context",
            raw: "0 libc.so.6 + 0x9929b eip = 0xf7da629b Found by: given as instruction pointer in context",
          },
        ],
      },
    ]);
  });

  it("ignores unrecognised output and empty separators", () => {
    expect(parseStackwalk("\nheader\n\nnot a frame\n")).toEqual([]);
  });

  it("selects the first frame from the crashed thread and formats its call", () => {
    const threads = parseStackwalk([
      "Thread 0",
      "0 worker.so!Idle + 0x8",
      "Thread 2 (crashed)",
      "0 server.so!Crash + 0x10",
    ].join("\n"));
    const frame = getCrashedThreadTopFrame(threads);
    expect(frame).toMatchObject({ index: "0", module: "server.so", symbol: "Crash", offset: "0x10" });
    expect(formatStackwalkFrame(frame!)).toBe("server.so!Crash + 0x10");
  });

  it("falls back to the first non-empty thread when the crashed thread has no frames", () => {
    const threads = parseStackwalk([
      "Thread 2 (crashed)",
      "Thread 3",
      "0 server.so!Crash + 0x10",
    ].join("\n"));

    expect(getCrashedThreadTopFrame(threads)).toMatchObject({ module: "server.so", symbol: "Crash" });
  });
});

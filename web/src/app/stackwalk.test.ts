import { describe, expect, it } from "vitest";
import { parseStackwalk } from "./stackwalk";

describe("parseStackwalk", () => {
  it("splits compact frames into module, symbol and offset", () => {
    expect(parseStackwalk("#0 server!Crash+0x10\n#1 engine!Run+0x20")).toEqual([
      { kind: "frame", index: "0", module: "server", symbol: "Crash", offset: "0x10", raw: "#0 server!Crash+0x10" },
      { kind: "frame", index: "1", module: "engine", symbol: "Run", offset: "0x20", raw: "#1 engine!Run+0x20" },
    ]);
  });

  it("keeps real stackwalk source annotations and explanatory lines", () => {
    expect(parseStackwalk([
      "Thread 0 (crashed)",
      "0 libc.so.6 + 0x9929b eip = 0xf7da629b",
      "    Found by: given as instruction pointer in context",
      "3 libtier0_srv.so!Plat_FloatTime + 0x2b eip = 0xf7cca02b",
      "    Found by: stack scanning",
      "warning: missing symbols",
    ].join("\n"))).toEqual([
      { kind: "log", raw: "Thread 0 (crashed)" },
      { kind: "frame", index: "0", module: "libc.so.6", offset: "0x9929b", foundBy: "given as instruction pointer in context", raw: "0 libc.so.6 + 0x9929b eip = 0xf7da629b\n    Found by: given as instruction pointer in context" },
      { kind: "frame", index: "3", module: "libtier0_srv.so", symbol: "Plat_FloatTime", offset: "0x2b", foundBy: "stack scanning", raw: "3 libtier0_srv.so!Plat_FloatTime + 0x2b eip = 0xf7cca02b\n    Found by: stack scanning" },
      { kind: "log", raw: "warning: missing symbols" },
    ]);
  });

  it("extracts a source annotation appended to the frame line", () => {
    expect(parseStackwalk("0 libc.so.6 + 0x9929b eip = 0xf7da629b Found by: given as instruction pointer in context")).toEqual([
      {
        kind: "frame",
        index: "0",
        module: "libc.so.6",
        offset: "0x9929b",
        foundBy: "given as instruction pointer in context",
        raw: "0 libc.so.6 + 0x9929b eip = 0xf7da629b Found by: given as instruction pointer in context",
      },
    ]);
  });

  it("keeps unrecognised output while ignoring empty separators", () => {
    expect(parseStackwalk("\nheader\n\nnot a frame\n")).toEqual([
      { kind: "log", raw: "header" },
      { kind: "log", raw: "not a frame" },
    ]);
  });
});

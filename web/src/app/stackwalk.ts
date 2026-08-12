export type StackwalkFrame = {
  kind: "frame";
  index: string;
  module: string;
  symbol?: string;
  offset?: string;
  foundBy?: string;
  raw: string;
};

export type StackwalkLogLine = {
  kind: "log";
  raw: string;
};

export type StackwalkEntry = StackwalkFrame | StackwalkLogLine;

const framePattern = /^\s*#?(\d+)\s+(.+?)(?=\s+eip\s*=|\s+Found by:|$)/i;
const offsetPattern = /\s*\+\s*(0x[0-9a-f]+)\s*$/i;
const foundByPattern = /^\s*Found by:\s*(.+?)\s*$/i;
const inlineFoundByPattern = /\s+Found by:\s*(.+?)\s*$/i;

function parseFrame(raw: string): StackwalkFrame | null {
  const match = raw.match(framePattern);
  if (!match) return null;

  const location = match[2].trim();
  const offsetMatch = location.match(offsetPattern);
  const locationWithoutOffset = offsetMatch ? location.slice(0, offsetMatch.index ?? location.length).trim() : location;
  const separator = locationWithoutOffset.indexOf("!");
  const module = (separator >= 0 ? locationWithoutOffset.slice(0, separator) : locationWithoutOffset).trim();
  const symbol = separator >= 0 ? locationWithoutOffset.slice(separator + 1).trim() : "";
  const inlineFoundBy = raw.match(inlineFoundByPattern)?.[1]?.trim();

  return {
    kind: "frame",
    index: match[1],
    module: module || "未知模块",
    ...(symbol ? { symbol } : {}),
    ...(offsetMatch ? { offset: offsetMatch[1] } : {}),
    ...(inlineFoundBy ? { foundBy: inlineFoundBy } : {}),
    raw,
  };
}

export function parseStackwalk(input: string): StackwalkEntry[] {
  const entries: StackwalkEntry[] = [];
  let lastFrame: StackwalkFrame | undefined;

  for (const raw of input.split(/\r?\n/)) {
    if (!raw.trim()) continue;

    const frame = parseFrame(raw);
    if (frame) {
      entries.push(frame);
      lastFrame = frame;
      continue;
    }

    const foundBy = raw.match(foundByPattern);
    if (foundBy && lastFrame) {
      lastFrame.foundBy = foundBy[1].trim();
      lastFrame.raw = `${lastFrame.raw}\n${raw}`;
      continue;
    }

    entries.push({ kind: "log", raw });
    lastFrame = undefined;
  }

  return entries;
}

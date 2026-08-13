export type StackwalkFrame = {
  kind: "frame";
  index: string;
  module: string;
  symbol?: string;
  offset?: string;
  foundBy?: string;
  raw: string;
};

export type StackwalkThread = {
  id: string;
  crashed: boolean;
  frames: StackwalkFrame[];
};

const framePattern = /^\s*#?(\d+)\s+(.+?)(?=\s+eip\s*=|\s+Found by:|$)/i;
const offsetPattern = /\s*\+\s*(0x[0-9a-f]+)\s*$/i;
const sourceLocationPattern = /\s+\[[^\]]*?(?:\+\s*(0x[0-9a-f]+))?\]\s*$/i;
const foundByPattern = /^\s*Found by:\s*(.+?)\s*$/i;
const inlineFoundByPattern = /\s+Found by:\s*(.+?)\s*$/i;
const registerContinuationPattern = /^\s+(?:eip|esp|ebp|eax|ebx|ecx|edx|esi|edi|efl|cs|ss|ds|es|fs|gs)\s*=/i;
const threadPattern = /^\s*Thread\s+(\d+)(?:\s+\(([^)]+)\))?\s*$/i;

function parseFrame(raw: string): StackwalkFrame | null {
  const match = raw.match(framePattern);
  if (!match) return null;

  const location = match[2].trim();
  const sourceLocationMatch = location.match(sourceLocationPattern);
  const locationWithoutSource = sourceLocationMatch ? location.slice(0, sourceLocationMatch.index ?? location.length).trim() : location;
  const offsetMatch = locationWithoutSource.match(offsetPattern);
  const offset = offsetMatch?.[1] || sourceLocationMatch?.[1];
  const locationWithoutOffset = offsetMatch ? locationWithoutSource.slice(0, offsetMatch.index ?? locationWithoutSource.length).trim() : locationWithoutSource;
  const separator = locationWithoutOffset.indexOf("!");
  const module = (separator >= 0 ? locationWithoutOffset.slice(0, separator) : locationWithoutOffset).trim();
  const symbol = separator >= 0 ? locationWithoutOffset.slice(separator + 1).trim() : "";
  const inlineFoundBy = raw.match(inlineFoundByPattern)?.[1]?.trim();

  return {
    kind: "frame",
    index: match[1],
    module: module || "未知模块",
    ...(symbol ? { symbol } : {}),
    ...(offset ? { offset } : {}),
    ...(inlineFoundBy ? { foundBy: inlineFoundBy } : {}),
    raw,
  };
}

export function parseStackwalk(input: string): StackwalkThread[] {
  const threads: StackwalkThread[] = [];
  let current: StackwalkThread = { id: "0", crashed: false, frames: [] };
  let lastFrame: StackwalkFrame | undefined;

  const commitCurrent = () => {
    if (current.frames.length) threads.push(current);
  };

  for (const raw of input.split(/\r?\n/)) {
    if (!raw.trim()) continue;

    const thread = raw.match(threadPattern);
    if (thread) {
      commitCurrent();
      current = { id: thread[1], crashed: /\bcrashed\b/i.test(thread[2] || ""), frames: [] };
      lastFrame = undefined;
      continue;
    }

    const frame = parseFrame(raw);
    if (frame) {
      current.frames.push(frame);
      lastFrame = frame;
      continue;
    }

    const foundBy = raw.match(foundByPattern);
    if (foundBy && lastFrame) {
      lastFrame.foundBy = foundBy[1].trim();
      lastFrame.raw = `${lastFrame.raw}\n${raw}`;
      continue;
    }

    if (lastFrame && registerContinuationPattern.test(raw)) continue;

    lastFrame = undefined;
  }

  commitCurrent();
  return threads;
}

export function getCrashedThreadTopFrame(threads: StackwalkThread[]): StackwalkFrame | undefined {
  return threads.find((thread) => thread.crashed)?.frames[0] || threads.find((thread) => thread.frames.length)?.frames[0];
}

export function formatStackwalkFrame(frame: StackwalkFrame): string {
  const location = frame.symbol ? `${frame.module}!${frame.symbol}` : frame.module;
  return frame.offset ? `${location} + ${frame.offset}` : location;
}

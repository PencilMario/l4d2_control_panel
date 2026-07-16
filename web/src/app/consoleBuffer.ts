export const NATIVE_CONSOLE_MAX_LINES = 1000;

export function appendConsoleOutput(
  current: string,
  incoming: string,
  maxLines = NATIVE_CONSOLE_MAX_LINES,
) {
  if (maxLines <= 0) return "";
  const combined = current + incoming;
  let newlineCount = 0;
  for (let index = 0; index < combined.length; index += 1) {
    if (combined.charCodeAt(index) === 10) newlineCount += 1;
  }
  const lineCount = newlineCount + (combined.endsWith("\n") ? 0 : 1);
  let linesToDrop = lineCount - maxLines;
  if (linesToDrop <= 0) return combined;

  let offset = 0;
  while (linesToDrop > 0) {
    const newline = combined.indexOf("\n", offset);
    if (newline < 0) return combined;
    offset = newline + 1;
    linesToDrop -= 1;
  }
  return combined.slice(offset);
}

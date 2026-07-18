import React from 'react';

export type LogToken = { text: string; className?: string };

const ANSI_COLORS: Record<number, string> = {
  30: 'log-ansi-black', 31: 'log-ansi-red', 32: 'log-ansi-green', 33: 'log-ansi-yellow',
  34: 'log-ansi-blue', 35: 'log-ansi-magenta', 36: 'log-ansi-cyan', 37: 'log-ansi-white',
  90: 'log-ansi-bright-black', 91: 'log-ansi-bright-red', 92: 'log-ansi-bright-green', 93: 'log-ansi-bright-yellow',
  94: 'log-ansi-bright-blue', 95: 'log-ansi-bright-magenta', 96: 'log-ansi-bright-cyan', 97: 'log-ansi-bright-white',
};

function validAddress(value: string) {
  const unbracketed = value.replace(/^\[|\](?::\d+)?$/g, '');
  const ipv4 = unbracketed.match(/^(\d{1,3}(?:\.\d{1,3}){3})(?::(\d+))?$/);
  if (ipv4) return ipv4[1].split('.').every((part) => Number(part) <= 255) && (!ipv4[2] || Number(ipv4[2]) <= 65535);
  const port = value.match(/\]:(\d+)$/)?.[1];
  const colons = unbracketed.match(/:/g)?.length || 0;
  const looksIPv6 = unbracketed.includes('::') || (/[a-f]/i.test(unbracketed) && colons >= 2) || colons >= 4;
  return looksIPv6 && /^[0-9a-f:]+$/i.test(unbracketed) && (!port || Number(port) <= 65535);
}

export function tokenizeLog(input: string): LogToken[] {
  const output: LogToken[] = [];
  let color = '';
  let emphasis = false;
  const activeClass = () => [color, emphasis ? 'log-token-emphasis' : ''].filter(Boolean).join(' ') || undefined;
  const emit = (text: string) => {
    const regex = /(\b\d{4}-\d{2}-\d{2}[ T]\d{2}:\d{2}:\d{2}\b|\[\d{2}:\d{2}:\d{2}\])|(\[(?:plugin|module|source|sm)\]|\bplugin\s*:\s*[\w.-]+|\b(?:source|module)\/[\w.-]+)|(\b(?:ERROR|FATAL|WARN(?:ING)?|INFO|DEBUG|TRACE)\b)|(\b(?:SteamID|SteamId|UserID|userid)\s*[:=]?\s*[A-Za-z0-9_:.-]+)|(\b\d{1,3}(?:\.\d{1,3}){3}(?::\d{1,5})?\b|\[[0-9A-Fa-f:]+\](?::\d{1,5})?|(?<![\w:])[0-9A-Fa-f]*:[0-9A-Fa-f:]+(?![\w:]))|([\w./\\-]+\.(?:sp|cpp|c|h|inc):\d+)/gi;
    let cursor = 0;
    let match: RegExpExecArray | null;
    while ((match = regex.exec(text))) {
      if (match.index > cursor) output.push({ text: text.slice(cursor, match.index), className: activeClass() });
      let className = activeClass();
      if (match[1]) className = 'log-token-timestamp';
      else if (match[2]) className = /plugin|\[sm\]/i.test(match[2]) ? 'log-token-plugin' : 'log-token-module';
      else if (match[3]) className = match[3].toUpperCase().startsWith('WARN') ? 'log-token-warn' : match[3].toUpperCase() === 'INFO' ? 'log-token-info' : 'log-token-error';
      else if (match[4]) className = 'log-token-user';
      else if (match[5] && validAddress(match[5])) className = 'log-token-address';
      else if (match[6]) className = 'log-token-file';
      output.push({ text: match[0], className });
      cursor = match.index + match[0].length;
    }
    if (cursor < text.length) output.push({ text: text.slice(cursor), className: activeClass() });
  };
  const ansi = /\x1b\[([0-9;]*)m/g;
  let cursor = 0;
  let match: RegExpExecArray | null;
  while ((match = ansi.exec(input))) {
    emit(input.slice(cursor, match.index));
    const codes = (match[1] || '0').split(';').map(Number);
    let known = true;
    for (const code of codes) {
      if (code === 0) { color = ''; emphasis = false; }
      else if (code === 1) emphasis = true;
      else if (code === 22) emphasis = false;
      else if (code === 39) color = '';
      else if (ANSI_COLORS[code]) color = ANSI_COLORS[code];
      else known = false;
    }
    if (!known) emit(match[0]);
    cursor = match.index + match[0].length;
  }
  emit(input.slice(cursor));
  return output;
}

export const DISPLAY_PREVIEW_LIMIT = 1024 * 1024;

export function truncateForDisplay(text: string): { text: string; truncated: boolean } {
  const bytes = new TextEncoder().encode(text);
  if (bytes.byteLength <= DISPLAY_PREVIEW_LIMIT) return { text, truncated: false };
  let start = bytes.byteLength - DISPLAY_PREVIEW_LIMIT;
  while (start < bytes.byteLength && (bytes[start] & 0xc0) === 0x80) start++;
  return { text: new TextDecoder().decode(bytes.slice(start)), truncated: true };
}

export function HighlightedLog({ text }: { text: string }) {
  const display = truncateForDisplay(text);
  return <><pre className="log-viewer">{tokenizeLog(display.text).map((token, index) => <span key={index} className={token.className}>{token.text}</span>)}</pre>{display.truncated ? <p>Tail truncated to {DISPLAY_PREVIEW_LIMIT} bytes</p> : null}</>;
}

import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { DISPLAY_PREVIEW_LIMIT, HighlightedLog, MAX_RENDER_TOKENS, tokenizeLog, truncateForDisplay } from './logHighlight';

describe('log highlighting', () => {
  it('supports combined SGR emphasis, colors, and foreground reset', () => {
    const tokens = tokenizeLog('\x1b[1;31mbold red\x1b[39m bold\x1b[0m plain');
    expect(tokens.find((token) => token.text === 'bold red')?.className).toContain('log-ansi-red');
    expect(tokens.find((token) => token.text === 'bold red')?.className).toContain('log-token-emphasis');
    expect(tokens.find((token) => token.text === ' bold')?.className).toBe('log-token-emphasis');
    expect(tokens.find((token) => token.text === ' plain')?.className).toBeUndefined();
  });

  it('maps normal and bright ANSI foreground colors', () => {
    for (const code of [30, 31, 32, 33, 34, 35, 36, 37, 90, 91, 92, 93, 94, 95, 96, 97]) {
      expect(tokenizeLog(`\x1b[${code}mvalue\x1b[0m`)[0].className).toBeTruthy();
    }
    expect(tokenizeLog('\x1b[31mnormal\x1b[0m')[0].className).toBe('log-ansi-red');
    expect(tokenizeLog('\x1b[91mbright\x1b[0m')[0].className).toBe('log-ansi-bright-red');
  });

  it('recognizes bare IPv4, IPv6, optional ports, and common module/plugin forms', () => {
    const tokens = tokenizeLog('10.0.0.1 10.0.0.2:27015 2001:db8::1 [2001:db8::2]:27015 [SM] Plugin: Left4DHooks module/engine');
    expect(tokens.filter((token) => token.className === 'log-token-address')).toHaveLength(4);
    expect(tokens.some((token) => token.className === 'log-token-plugin')).toBe(true);
    expect(tokens.some((token) => token.className === 'log-token-module')).toBe(true);
  });

  it('does not over-highlight invalid address-like text', () => {
    expect(tokenizeLog('999.1.2.3:70000 abc:def and 12:30:01').some((token) => token.className === 'log-token-address')).toBe(false);
  });

  it('recognizes bare Steam IDs and keeps player names as independent tokens', () => {
    const tokens = tokenizeLog('connected "Player Name<7><STEAM_1:0:42><Survivor>" (STEAM_0:1:1234)');

    expect(tokens.filter((token) => token.className === 'log-token-player').map((token) => token.text)).toEqual(['Player Name']);
    expect(tokens.filter((token) => token.className === 'log-token-steamid').map((token) => token.text)).toEqual([
      'STEAM_1:0:42',
      'STEAM_0:1:1234',
    ]);
    expect(tokens.map((token) => token.text).join('')).toBe('connected "Player Name<7><STEAM_1:0:42><Survivor>" (STEAM_0:1:1234)');
  });

  it('highlights exception keywords without consuming the rest of the line', () => {
    const tokens = tokenizeLog('Exception: bad state; Error while loading; panic: stopped');

    expect(tokens.filter((token) => token.className === 'log-token-exception').map((token) => token.text)).toEqual([
      'Exception',
      'Error',
      'panic',
    ]);
    expect(tokens.some((token) => token.className === 'log-token-exception' && token.text.includes('bad state'))).toBe(false);
  });

  it('recognizes common call frames as bounded stack tokens', () => {
    const javascript = '    at loadPlugin (src/plugins/loader.ts:42:7)';
    const sourcePawn = '[3] addons/sourcemod/scripting/test.sp::OnPluginStart (line 128)';
    const tokens = tokenizeLog(`${javascript}\n${sourcePawn}\nordinary output remains plain`);

    expect(tokens.filter((token) => token.className === 'log-token-stack').map((token) => token.text)).toEqual([
      'at loadPlugin (src/plugins/loader.ts:42:7)',
      sourcePawn,
    ]);
    expect(tokens.find((token) => token.text.includes('ordinary output'))?.className).toBeUndefined();
  });

  it('truncates the UTF-8 tail to at most one MiB of bytes', () => {
    const result = truncateForDisplay('界'.repeat(DISPLAY_PREVIEW_LIMIT));
    expect(result.truncated).toBe(true);
    expect(new TextEncoder().encode(result.text).byteLength).toBeLessThanOrEqual(DISPLAY_PREVIEW_LIMIT);
    expect(result.text.endsWith('界')).toBe(true);
  });

  it('renders html-looking text as text', () => {
    render(<HighlightedLog text="<img src=x> unknown" />);
    expect(document.querySelector('pre')?.textContent).toContain('<img src=x>');
    expect(document.querySelector('img')).toBeNull();
  });

  it('bounds repeated semantic tokens and keeps the log tail visible', () => {
    const text = `${'INFO message\n'.repeat(90_000)}TAIL-MARKER`;
    const tokens = tokenizeLog(text);

    expect(tokens.length).toBeLessThanOrEqual(MAX_RENDER_TOKENS);
    expect(tokens.map((token) => token.text).join('')).toBe(text);
    expect(tokens.at(-1)?.text).toContain('TAIL-MARKER');

    const { getByText } = render(<HighlightedLog text={text} />);
    expect(getByText('高亮已简化')).toBeVisible();
  });
});

import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { DISPLAY_PREVIEW_LIMIT, HighlightedLog, tokenizeLog, truncateForDisplay } from './logHighlight';

describe('log highlighting', () => {
  it('supports combined SGR emphasis, colors, and foreground reset', () => {
    const tokens = tokenizeLog('\x1b[1;31mbold red\x1b[39m bold\x1b[0m plain');
    expect(tokens.find((token) => token.text === 'bold red')?.className).toContain('log-token-error');
    expect(tokens.find((token) => token.text === 'bold red')?.className).toContain('log-token-emphasis');
    expect(tokens.find((token) => token.text === ' bold')?.className).toBe('log-token-emphasis');
    expect(tokens.find((token) => token.text === ' plain')?.className).toBeUndefined();
  });

  it('maps normal and bright ANSI foreground colors', () => {
    for (const code of [30, 31, 32, 33, 34, 35, 36, 37, 90, 91, 92, 93, 94, 95, 96, 97]) {
      expect(tokenizeLog(`\x1b[${code}mvalue\x1b[0m`)[0].className).toBeTruthy();
    }
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
});

import { render, screen } from '@testing-library/react';
import { HighlightedLog, tokenizeLog } from './logHighlight';
import { describe, expect, it } from 'vitest';
describe('log highlighting',()=>{
 it('tokenizes ansi and semantic values safely',()=>{ const t=tokenizeLog('\x1b[31m[12:30:01] ERROR plugin steamid 1.2.3.4:27015 file.sp:42\x1b[0m'); expect(t.some(x=>x.className==='log-token-error')).toBe(true); });
 it('renders html-looking text as text',()=>{ render(<HighlightedLog text={'<img src=x> unknown\nline'}/>); expect(document.querySelector('pre')?.textContent).toContain('<img src=x> unknown'); expect(document.querySelector('img')).toBeNull(); });
 it('preserves unknown ansi and highlights timestamp module plugin',()=>{const t=tokenizeLog('2026-07-18 12:30:01 [plugin] source/module: hello \x1b[99mred'); expect(t.some(x=>x.className==='log-token-timestamp')).toBe(true); expect(t.some(x=>x.className==='log-token-plugin'||x.className==='log-token-module')).toBe(true); expect(t.map(x=>x.text).join('')).toContain('\x1b[99m');});
 it('highlights levels identities addresses and stack files',()=>{const t=tokenizeLog('[12:30:01] [module] ERROR SteamID:STEAM_1:1:123 UserID:42 192.168.1.2:27015 [2001:db8::1]:27015 src/foo.sp:42'); const c=t.map(x=>x.className); expect(c).toContain('log-token-module'); expect(c).toContain('log-token-error'); expect(c.filter(x=>x==='log-token-user').length).toBeGreaterThanOrEqual(2); expect(c).toContain('log-token-address'); expect(c).toContain('log-token-file');});
});

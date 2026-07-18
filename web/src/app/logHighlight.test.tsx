import { render, screen } from '@testing-library/react';
import { HighlightedLog, tokenizeLog } from './logHighlight';
import { describe, expect, it } from 'vitest';
describe('log highlighting',()=>{
 it('tokenizes ansi and semantic values safely',()=>{ const t=tokenizeLog('\x1b[31m[12:30:01] ERROR plugin steamid 1.2.3.4:27015 file.sp:42\x1b[0m'); expect(t.some(x=>x.className==='log-token-error')).toBe(true); });
 it('renders html-looking text as text',()=>{ render(<HighlightedLog text={'<img src=x> unknown\nline'}/>); expect(document.querySelector('pre')?.textContent).toContain('<img src=x> unknown'); expect(document.querySelector('img')).toBeNull(); });
});

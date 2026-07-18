import { render, screen, waitFor } from '@testing-library/react';
import { describe,it,expect,vi } from 'vitest';
import { GameLogsPage } from './GameLogsPage';
describe('GameLogsPage',()=>{ it('loads tree and preview metadata',async()=>{ const api=vi.fn().mockResolvedValueOnce({entries:[{path:'game/a.log',kind:'file',size:12,modified_at:'now'}]}).mockResolvedValueOnce({text:'INFO hi',size:8,modified_at:'now',truncated:false}); render(<GameLogsPage instanceID="i1" api={api}/>); await waitFor(()=>expect(screen.getByText('game/a.log')).toBeTruthy()); expect(api).toHaveBeenCalled(); }); });

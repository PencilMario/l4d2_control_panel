import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { describe,it,expect,vi } from 'vitest';
import { GameLogsPage } from './GameLogsPage';
describe('GameLogsPage',()=>{ it('loads tree and preview metadata',async()=>{ const api=vi.fn().mockResolvedValueOnce({entries:[{path:'game/a.log',kind:'file',size:12,modified_at:'now'}]}).mockResolvedValueOnce({text:'INFO hi',size:8,modified_at:'now',truncated:false}); render(<GameLogsPage instanceID="i1" api={api}/>); await waitFor(()=>expect(screen.getByText('game/a.log')).toBeTruthy()); expect(api).toHaveBeenCalled(); }); });
it('renders nested directories collapsed then expands',async()=>{const api=vi.fn().mockResolvedValue({entries:[{path:'game/logs',kind:'directory'},{path:'game/logs/a',kind:'directory'},{path:'game/logs/a/x.log',kind:'file'}]});render(<GameLogsPage instanceID="i" api={api}/>);await waitFor(()=>expect(screen.getByRole('heading',{name:'game'})).toBeTruthy());const logs=await waitFor(()=>screen.getByLabelText('Toggle game/logs'));expect(screen.queryByText('game/logs/a/x.log')).toBeNull();fireEvent.click(logs);const nested=await waitFor(()=>screen.getByLabelText('Toggle game/logs/a'));fireEvent.click(nested);await waitFor(()=>expect(screen.getByText('game/logs/a/x.log')).toBeTruthy())});
it('uses sourcemod kind in download',async()=>{const api=vi.fn().mockResolvedValueOnce({entries:[{path:'sourcemod/a.log',kind:'file'}]}).mockResolvedValueOnce({text:'x'});render(<GameLogsPage instanceID="i" api={api}/>);await waitFor(()=>screen.getByLabelText('sourcemod/a.log').click());await waitFor(()=>expect(screen.getByLabelText('Download').getAttribute('href')).toContain('kind=sourcemod'))});





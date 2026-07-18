import { fireEvent, render, screen, waitFor } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { GameLogsPage } from './GameLogsPage';

const entry = { kind: 'game', path: 'logs/archive/a.log', size: 12, modified_at: '2026-07-18T10:00:00Z' };

afterEach(() => vi.restoreAllMocks());

describe('GameLogsPage', () => {
  it('consumes the bare Entry[] contract and builds directories from relative paths', async () => {
    const api = vi.fn().mockResolvedValue([entry]);
    render(<GameLogsPage instanceID="i1" api={api} />);
    const logs = await waitFor(() => screen.getByLabelText('Toggle game/logs'));
    expect(screen.queryByLabelText('logs/archive/a.log')).toBeNull();
    fireEvent.click(logs);
    fireEvent.click(screen.getByLabelText('Toggle game/logs/archive'));
    expect(screen.getByLabelText('logs/archive/a.log')).toBeTruthy();
  });

  it('shows selected file metadata', async () => {
    const api = vi.fn().mockResolvedValueOnce([entry]).mockResolvedValueOnce({ text: 'INFO hi', size: 12, modified_at: entry.modified_at });
    render(<GameLogsPage instanceID="i1" api={api} />);
    fireEvent.click(await waitFor(() => screen.getByLabelText('Toggle game/logs')));
    fireEvent.click(screen.getByLabelText('Toggle game/logs/archive'));
    fireEvent.click(screen.getByLabelText(entry.path));
    await waitFor(() => expect(screen.getByText(/12 B/)).toBeTruthy());
    expect(screen.getByText(/2026/)).toBeTruthy();
  });

  it('refreshes both tree and the current preview when the file still exists', async () => {
    const api = vi.fn()
      .mockResolvedValueOnce([{ ...entry, path: 'a.log' }])
      .mockResolvedValueOnce({ text: 'old' })
      .mockResolvedValueOnce([{ ...entry, path: 'a.log', size: 20 }])
      .mockResolvedValueOnce({ text: 'new' });
    render(<GameLogsPage instanceID="i" api={api} />);
    fireEvent.click(await waitFor(() => screen.getByLabelText('a.log')));
    await waitFor(() => screen.getByText('old'));
    fireEvent.click(screen.getByLabelText('Refresh'));
    await waitFor(() => screen.getByText('new'));
    expect(api).toHaveBeenCalledTimes(4);
  });

  it('downloads through the blob API and reports a local failure', async () => {
    const api = vi.fn().mockResolvedValueOnce([{ ...entry, path: 'a.log' }]).mockResolvedValueOnce({ text: 'x' });
    const blobApi = vi.fn().mockRejectedValue(new Error('download failed'));
    render(<GameLogsPage instanceID="i" api={api} blobApi={blobApi} />);
    fireEvent.click(await waitFor(() => screen.getByLabelText('a.log')));
    fireEvent.click(await waitFor(() => screen.getByLabelText('Download')));
    await waitFor(() => expect(screen.getByRole('alert')).toHaveTextContent('download failed'));
  });

  it('downloads a successful blob with the selected filename', async () => {
    const api = vi.fn().mockResolvedValueOnce([{ ...entry, path: 'a.log' }]).mockResolvedValueOnce({ text: 'x' });
    const blobApi = vi.fn().mockResolvedValue(new Blob(['x']));
    const createObjectURL = vi.fn(() => 'blob:test');
    const revokeObjectURL = vi.fn();
    Object.defineProperty(URL, 'createObjectURL', { configurable: true, value: createObjectURL });
    Object.defineProperty(URL, 'revokeObjectURL', { configurable: true, value: revokeObjectURL });
    const click = vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
    render(<GameLogsPage instanceID="i" api={api} blobApi={blobApi} />);
    fireEvent.click(await waitFor(() => screen.getByLabelText('a.log')));
    fireEvent.click(await waitFor(() => screen.getByLabelText('Download')));
    await waitFor(() => expect(blobApi).toHaveBeenCalledWith(expect.stringContaining('kind=game')));
    expect(click).toHaveBeenCalled();
    expect(createObjectURL).toHaveBeenCalled();
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:test');
  });

  it('renders an accessible mobile drawer only while open and restores focus on Escape', async () => {
    const api = vi.fn().mockResolvedValue([]);
    render(<GameLogsPage instanceID="i" api={api} />);
    const trigger = screen.getByRole('button', { name: 'Open log tree' });
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('dialog', { name: 'Log tree' })).toBeNull();
    fireEvent.click(trigger);
    expect(trigger).toHaveAttribute('aria-expanded', 'true');
    const drawer = screen.getByRole('dialog', { name: 'Log tree' });
    expect(drawer).toHaveAttribute('aria-modal', 'true');
    await waitFor(() => expect(screen.getByRole('button', { name: 'Close log tree' })).toHaveFocus());
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    expect(screen.queryByRole('dialog', { name: 'Log tree' })).toBeNull();
    expect(trigger).toHaveFocus();
  });

  it('closes the mobile drawer from its overlay and restores focus', async () => {
    const api = vi.fn().mockResolvedValue([]);
    render(<GameLogsPage instanceID="i" api={api} />);
    const trigger = screen.getByRole('button', { name: 'Open log tree' });
    fireEvent.click(trigger);
    await waitFor(() => expect(screen.getByRole('dialog', { name: 'Log tree' })).toBeTruthy());
    fireEvent.click(screen.getByTestId('game-logs-drawer-overlay'));
    expect(screen.queryByRole('dialog', { name: 'Log tree' })).toBeNull();
    expect(trigger).toHaveFocus();
  });

  it('uses the sourcemod category in preview and download requests', async () => {
    const sourcemod = { ...entry, kind: 'sourcemod' as const, path: 'addons/sourcemod/logs/errors.log' };
    const api = vi.fn().mockResolvedValueOnce([sourcemod]).mockResolvedValueOnce({ text: 'x' });
    const blobApi = vi.fn().mockRejectedValue(new Error('stop'));
    render(<GameLogsPage instanceID="i" api={api} blobApi={blobApi} />);
    fireEvent.click(await waitFor(() => screen.getByLabelText('Toggle sourcemod/addons')));
    fireEvent.click(screen.getByLabelText('Toggle sourcemod/addons/sourcemod'));
    fireEvent.click(screen.getByLabelText('Toggle sourcemod/addons/sourcemod/logs'));
    fireEvent.click(screen.getByLabelText(sourcemod.path));
    await waitFor(() => expect(api).toHaveBeenLastCalledWith(expect.stringContaining('kind=sourcemod'), expect.anything()));
    fireEvent.click(screen.getByLabelText('Download'));
    await waitFor(() => expect(blobApi).toHaveBeenCalledWith(expect.stringContaining('kind=sourcemod')));
  });

  it('shows tree and preview error states including rotated files', async () => {
    const api = vi.fn().mockResolvedValueOnce([{ ...entry, path: 'a.log' }]).mockRejectedValueOnce(new Error('404 not found'));
    render(<GameLogsPage instanceID="i" api={api} />);
    fireEvent.click(await waitFor(() => screen.getByLabelText('a.log')));
    await waitFor(() => expect(screen.getByText('Log rotated or deleted')).toBeTruthy());
  });

  it('keeps only the latest preview when selections race', async () => {
    let firstResolve!: (value: unknown) => void;
    let secondResolve!: (value: unknown) => void;
    const api = vi.fn()
      .mockResolvedValueOnce([{ ...entry, path: 'a.log' }, { ...entry, path: 'b.log' }])
      .mockImplementationOnce(() => new Promise((resolve) => { firstResolve = resolve; }))
      .mockImplementationOnce(() => new Promise((resolve) => { secondResolve = resolve; }));
    render(<GameLogsPage instanceID="i" api={api} />);
    fireEvent.click(await waitFor(() => screen.getByLabelText('a.log')));
    fireEvent.click(screen.getByLabelText('b.log'));
    secondResolve({ text: 'LATEST' });
    await waitFor(() => screen.getByText('LATEST'));
    firstResolve({ text: 'STALE' });
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(screen.queryByText('STALE')).toBeNull();
  });

  it('shows the server truncation notice', async () => {
    const api = vi.fn().mockResolvedValueOnce([{ ...entry, path: 'a.log' }]).mockResolvedValueOnce({ text: 'tail', truncated: true });
    render(<GameLogsPage instanceID="i" api={api} />);
    fireEvent.click(await waitFor(() => screen.getByLabelText('a.log')));
    await waitFor(() => expect(screen.getByText(`Tail truncated to ${10 * 1024 * 1024} bytes`)).toBeTruthy());
  });
});

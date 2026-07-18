import { act, fireEvent, render, screen, waitFor, within } from '@testing-library/react';
import { afterEach, describe, expect, it, vi } from 'vitest';
import { GameLogsPage } from './GameLogsPage';

const entry = { kind: 'game', path: 'logs/archive/a.log', size: 12, modified_at: '2026-07-18T10:00:00Z' };
const instances = (id = 'i') => [{ id, name: id }];

afterEach(() => vi.restoreAllMocks());

describe('GameLogsPage', () => {
  it('uses the private-files page contract with an in-page target instance selector', async () => {
    const api = vi.fn().mockResolvedValue([]);
    const instances = [
      { id: 'i1', name: '死亡中心' },
      { id: 'i2', name: '黑色狂欢节' },
    ];

    render(<GameLogsPage instances={instances} api={api} />);

    expect(screen.getByRole('heading', { name: '游戏日志', level: 2 })).toBeVisible();
    expect(screen.getByRole('combobox', { name: '目标实例' })).toHaveValue('i1');
    expect(screen.getByRole('toolbar', { name: '游戏日志工具栏' })).toHaveClass('private-toolbar');
    expect(screen.getByRole('complementary', { name: '游戏日志目录' })).toHaveClass('private-tree-pane');
    expect(screen.getByLabelText('日志查看区')).toHaveClass('private-workspace');
    await waitFor(() => expect(api).toHaveBeenCalledWith('/api/instances/i1/game-logs/tree', expect.anything()));
  });

  it('consumes the bare Entry[] contract and builds directories from relative paths', async () => {
    const api = vi.fn().mockResolvedValue([entry]);
    render(<GameLogsPage instances={instances('i1')} api={api} />);
    const logs = await waitFor(() => screen.getByLabelText('Toggle game/logs'));
    expect(screen.queryByLabelText('logs/archive/a.log')).toBeNull();
    fireEvent.click(logs);
    fireEvent.click(screen.getByLabelText('Toggle game/logs/archive'));
    expect(screen.getByLabelText('logs/archive/a.log')).toBeTruthy();
  });

  it('shows selected file metadata', async () => {
    const api = vi.fn().mockResolvedValueOnce([entry]).mockResolvedValueOnce({ text: 'INFO hi', size: 12, modified_at: entry.modified_at });
    render(<GameLogsPage instances={instances('i1')} api={api} />);
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
    render(<GameLogsPage instances={instances()} api={api} />);
    fireEvent.click(await waitFor(() => screen.getByLabelText('a.log')));
    await waitFor(() => screen.getByText('old'));
    fireEvent.click(screen.getByRole('button', { name: '刷新' }));
    await waitFor(() => screen.getByText('new'));
    expect(api).toHaveBeenCalledTimes(4);
  });

  it('downloads through the blob API and reports a local failure', async () => {
    const api = vi.fn().mockResolvedValueOnce([{ ...entry, path: 'a.log' }]).mockResolvedValueOnce({ text: 'x' });
    const blobApi = vi.fn().mockRejectedValue(new Error('download failed'));
    render(<GameLogsPage instances={instances()} api={api} blobApi={blobApi} />);
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
    render(<GameLogsPage instances={instances()} api={api} blobApi={blobApi} />);
    fireEvent.click(await waitFor(() => screen.getByLabelText('a.log')));
    fireEvent.click(await waitFor(() => screen.getByLabelText('Download')));
    await waitFor(() => expect(blobApi).toHaveBeenCalledWith(expect.stringContaining('kind=game')));
    expect(click).toHaveBeenCalled();
    expect(createObjectURL).toHaveBeenCalled();
    expect(revokeObjectURL).toHaveBeenCalledWith('blob:test');
  });

  it('uses the private-files mobile drawer contract and restores focus on Escape', async () => {
    const api = vi.fn().mockResolvedValue([]);
    render(<GameLogsPage instances={instances()} api={api} />);
    const trigger = screen.getByRole('button', { name: '打开文件树' });
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    expect(document.getElementById('game-logs-drawer')).toHaveAttribute('aria-hidden', 'true');
    fireEvent.click(trigger);
    expect(trigger).toHaveAttribute('aria-expanded', 'true');
    const drawer = screen.getByRole('dialog', { name: '游戏日志目录' });
    expect(drawer).toHaveAttribute('aria-modal', 'true');
    await waitFor(() => expect(screen.getByRole('button', { name: '关闭文件树' })).toHaveFocus());
    fireEvent.keyDown(document, { key: 'Escape' });
    expect(trigger).toHaveAttribute('aria-expanded', 'false');
    expect(document.getElementById('game-logs-drawer')).toHaveAttribute('aria-hidden', 'true');
    expect(trigger).toHaveFocus();
  });

  it('closes the mobile drawer from the same close control as private files', async () => {
    const api = vi.fn().mockResolvedValue([]);
    render(<GameLogsPage instances={instances()} api={api} />);
    const trigger = screen.getByRole('button', { name: '打开文件树' });
    fireEvent.click(trigger);
    await waitFor(() => expect(screen.getByRole('dialog', { name: '游戏日志目录' })).toBeTruthy());
    fireEvent.click(screen.getByRole('button', { name: '关闭文件树' }));
    expect(document.getElementById('game-logs-drawer')).toHaveAttribute('aria-hidden', 'true');
    expect(trigger).toHaveFocus();
  });

  it('loops focus between the first and last controls in the mobile drawer', async () => {
    const api = vi.fn().mockResolvedValue([{ ...entry, path: 'a.log' }]);
    render(<GameLogsPage instances={instances()} api={api} />);
    await waitFor(() => expect(screen.getByLabelText('a.log')).toBeTruthy());
    fireEvent.click(screen.getByRole('button', { name: '打开文件树' }));
    const drawer = screen.getByRole('dialog', { name: '游戏日志目录' });
    const first = within(drawer).getByRole('button', { name: '关闭文件树' });
    const last = within(drawer).getByRole('treeitem', { name: 'a.log' });

    last.focus();
    fireEvent.keyDown(document, { key: 'Tab' });
    expect(first).toHaveFocus();

    first.focus();
    fireEvent.keyDown(document, { key: 'Tab', shiftKey: true });
    expect(last).toHaveFocus();
  });

  it('clears the old instance preview immediately and reloads the same path only after selection', async () => {
    let resolveNewTree!: (value: unknown) => void;
    const request = vi.fn((url: string) => {
      if (url === '/api/instances/old/game-logs/tree') return Promise.resolve([{ ...entry, path: 'same.log' }]);
      if (url.includes('/api/instances/old/game-logs/preview')) return Promise.resolve({ text: 'OLD CONTENT' });
      if (url === '/api/instances/new/game-logs/tree') return new Promise((resolve) => { resolveNewTree = resolve; });
      if (url.includes('/api/instances/new/game-logs/preview')) return Promise.resolve({ text: 'NEW CONTENT' });
      return Promise.reject(new Error(`Unexpected request: ${url}`));
    });
    const api = <T,>(url: string): Promise<T> => request(url) as Promise<T>;
    render(<GameLogsPage instances={[{ id: 'old', name: '旧实例' }, { id: 'new', name: '新实例' }]} api={api} />);
    fireEvent.click(await waitFor(() => screen.getByLabelText('same.log')));
    await waitFor(() => expect(screen.getByText('OLD CONTENT')).toBeTruthy());

    fireEvent.change(screen.getByRole('combobox', { name: '目标实例' }), { target: { value: 'new' } });
    expect(screen.queryByText('OLD CONTENT')).toBeNull();
    expect(screen.queryByLabelText('same.log')).toBeNull();

    await act(async () => resolveNewTree([{ ...entry, path: 'same.log' }]));
    const newEntry = await waitFor(() => screen.getByLabelText('same.log'));
    expect(screen.queryByText('NEW CONTENT')).toBeNull();
    fireEvent.click(newEntry);
    await waitFor(() => expect(screen.getByText('NEW CONTENT')).toBeTruthy());
  });

  it('uses the sourcemod category in preview and download requests', async () => {
    const sourcemod = { ...entry, kind: 'sourcemod' as const, path: 'addons/sourcemod/logs/errors.log' };
    const api = vi.fn().mockResolvedValueOnce([sourcemod]).mockResolvedValueOnce({ text: 'x' });
    const blobApi = vi.fn().mockRejectedValue(new Error('stop'));
    render(<GameLogsPage instances={instances()} api={api} blobApi={blobApi} />);
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
    render(<GameLogsPage instances={instances()} api={api} />);
    fireEvent.click(await waitFor(() => screen.getByLabelText('a.log')));
    await waitFor(() => expect(screen.getByText('日志已轮转或删除，请刷新目录')).toBeTruthy());
  });

  it('keeps only the latest preview when selections race', async () => {
    let firstResolve!: (value: unknown) => void;
    let secondResolve!: (value: unknown) => void;
    const api = vi.fn()
      .mockResolvedValueOnce([{ ...entry, path: 'a.log' }, { ...entry, path: 'b.log' }])
      .mockImplementationOnce(() => new Promise((resolve) => { firstResolve = resolve; }))
      .mockImplementationOnce(() => new Promise((resolve) => { secondResolve = resolve; }));
    render(<GameLogsPage instances={instances()} api={api} />);
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
    render(<GameLogsPage instances={instances()} api={api} />);
    fireEvent.click(await waitFor(() => screen.getByLabelText('a.log')));
    await waitFor(() => expect(screen.getByText(`仅显示文件末尾 ${10 * 1024 * 1024} 字节`)).toBeTruthy());
  });
});

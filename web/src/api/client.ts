export type Job = {
  ID: string;
  Status: string;
  Stage: string;
  Percent: number;
  Error: string;
};
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await fetch(path, {
    credentials: "same-origin",
    headers: { "Content-Type": "application/json", ...(init.headers || {}) },
    ...init,
  });
  if (!response.ok) {
    let message = `HTTP ${response.status}`;
    try {
      const body = await response.json();
      message = body.error?.message || message;
    } catch {}
    throw new Error(message);
  }
  if (response.status === 204) return undefined as T;
  return response.json();
}
export const normalizeInstance = (value: any) => ({
  id: value.id ?? value.ID,
  name: value.name ?? value.Name,
  actual_state: value.actual_state ?? value.ActualState,
  game_port: value.game_port ?? value.GamePort,
  start_map: value.start_map ?? value.StartMap,
  game_mode: value.game_mode ?? value.GameMode,
  max_players: value.max_players ?? value.MaxPlayers,
  players: value.players ?? 0,
  cpu: value.cpu ?? 0,
  memory: value.memory ?? 0,
});

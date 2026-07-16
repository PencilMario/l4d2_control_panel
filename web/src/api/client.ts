export type Job = {
  ID: string;
  InstanceID?: string;
  Type?: string;
  Status: string;
  Stage: string;
  Percent: number;
  Message?: string;
  Error: string;
  CreatedAt?: string;
  UpdatedAt?: string;
  StartedAt?: string | null;
  FinishedAt?: string | null;
  Events?: JobEvent[];
};
export type JobEvent = {
  ID: number;
  JobID: string;
  Kind: string;
  Stage: string;
  Percent: number;
  Message: string;
  CreatedAt: string;
};
export type JobLogLevel = "output" | "info" | "warn" | "error";
export type JobLogRecord = {
  seq: number;
  timestamp: string;
  source: string;
  level: JobLogLevel;
  message: string;
};
export type JobLogPage = {
  records: JobLogRecord[];
  next_seq: number;
  truncated: boolean;
};
async function request(path: string, init: RequestInit) {
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
  return response;
}
export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const response = await request(path, init);
  if (response.status === 204) return undefined as T;
  return response.json();
}
export async function apiText(
  path: string,
  init: RequestInit = {},
): Promise<string> {
  const response = await request(path, init);
  return response.text();
}
export async function apiBlob(
  path: string,
  init: RequestInit = {},
): Promise<Blob> {
  const response = await request(path, init);
  return response.blob();
}
export const normalizeInstance = (value: any) => ({
  id: value.id ?? value.ID,
  name: value.name ?? value.Name,
  actual_state: value.actual_state ?? value.ActualState,
  game_port: value.game_port ?? value.GamePort,
  sourcetv_port: value.sourcetv_port ?? value.SourceTVPort ?? 0,
  plugin_ports: value.plugin_ports ?? value.PluginPorts ?? [],
  start_map: value.start_map ?? value.StartMap,
  game_mode: value.game_mode ?? value.GameMode,
  tickrate: value.tickrate ?? value.Tickrate ?? 100,
  max_players: value.max_players ?? value.MaxPlayers,
  extra_args: value.extra_args ?? value.ExtraArgs ?? "",
  package_id: value.package_id ?? value.SelectedPackageID ?? "",
  applied_package_id:
    value.applied_package_id ?? value.PackageVersion ?? "",
  players: value.players ?? null,
  cpu: value.cpu ?? null,
  memory: value.memory ?? null,
});

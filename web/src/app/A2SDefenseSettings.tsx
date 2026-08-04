import { useEffect, useState, type FormEvent } from "react";
import { RefreshCw, Save, ShieldCheck } from "lucide-react";
import { api } from "../api/client";

type DefenseCounters = {
  info: number;
  player: number;
  rules: number;
  challenge: number;
  other_69: number;
  aggregate: number;
  blacklist: number;
};

type DefenseSettings = {
  desired_enabled: boolean;
  effective_enabled: boolean;
  pending: boolean;
  compatible: boolean;
  revision: number;
  policy_version: number;
  protected_ports: number[];
  counters: DefenseCounters;
  blacklist_size: number;
  applied_at?: string;
  last_error?: string;
};

const emptyCounters: DefenseCounters = { info: 0, player: 0, rules: 0, challenge: 0, other_69: 0, aggregate: 0, blacklist: 0 };

function normalizeSettings(value: Partial<DefenseSettings>): DefenseSettings {
  return {
    desired_enabled: value.desired_enabled === true,
    effective_enabled: value.effective_enabled === true,
    pending: value.pending === true,
    compatible: value.compatible === true,
    revision: Number.isSafeInteger(value.revision) ? value.revision! : 0,
    policy_version: Number.isSafeInteger(value.policy_version) ? value.policy_version! : 0,
    protected_ports: Array.isArray(value.protected_ports) ? value.protected_ports.filter(Number.isSafeInteger) : [],
    counters: { ...emptyCounters, ...(value.counters ?? {}) },
    blacklist_size: Number.isSafeInteger(value.blacklist_size) ? value.blacklist_size! : 0,
    applied_at: value.applied_at,
    last_error: value.last_error,
  };
}

const counterLabels: Array<[keyof DefenseCounters, string]> = [
  ["info", "A2S_INFO"],
  ["player", "A2S_PLAYER"],
  ["rules", "A2S_RULES"],
  ["challenge", "Challenge"],
  ["aggregate", "端口总限速"],
  ["blacklist", "黑名单"],
];

export function A2SDefenseSettings() {
  const [confirmed, setConfirmed] = useState<DefenseSettings | null>(null);
  const [draftEnabled, setDraftEnabled] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");

  useEffect(() => {
    void api<Partial<DefenseSettings>>("/api/settings/a2s-defense")
      .then((value) => {
        const settings = normalizeSettings(value);
        setConfirmed(settings);
        setDraftEnabled(settings.desired_enabled);
      })
      .catch((reason) => setError(reason instanceof Error ? reason.message : String(reason)));
  }, []);

  async function save(event: FormEvent) {
    event.preventDefault();
    if (!confirmed || busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const response = await api<Partial<DefenseSettings>>("/api/settings/a2s-defense", {
        method: "PUT",
        body: JSON.stringify({ enabled: draftEnabled }),
      });
      const saved = normalizeSettings(response);
      setConfirmed(saved);
      setDraftEnabled(saved.desired_enabled);
      setNotice(saved.desired_enabled ? "防御规则已启用" : "防御规则已停用");
    } catch (reason) {
      setDraftEnabled(confirmed.desired_enabled);
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      setBusy(false);
    }
  }

  const stateLabel = !confirmed
    ? "正在读取"
    : confirmed.pending
      ? "等待对账"
      : !confirmed.compatible
        ? "环境不兼容"
        : confirmed.effective_enabled
          ? "防护中"
          : "已停用";
  const stateClass = confirmed?.effective_enabled && !confirmed.pending ? "configured" : "unconfigured";

  return <section className="settings-card a2s-defense-settings" aria-labelledby="a2s-defense-settings-title">
    <div className="settings-card-title">
      <h3 id="a2s-defense-settings-title"><ShieldCheck />A2S 查询防御</h3>
      <span className={stateClass}>{stateLabel}</span>
    </div>
    {error || confirmed?.last_error ? <p className="error-text" role="alert">{error || confirmed?.last_error}</p> : null}
    <form className="settings-fields" onSubmit={save}>
      <label className="settings-toggle">
        <input type="checkbox" aria-label="启用 A2S 查询防御" checked={draftEnabled} disabled={!confirmed || busy || !confirmed.compatible} onChange={(event) => setDraftEnabled(event.target.checked)} />
        启用 IPv4 A2S 防御
      </label>
      <div className="a2s-defense-ports" aria-label="受保护端口">
        {(confirmed?.protected_ports ?? []).map((port) => <code key={port}>{port}</code>)}
        {confirmed && confirmed.protected_ports.length === 0 ? <span>无活动端口</span> : null}
      </div>
      <div className="a2s-defense-counters">
        {counterLabels.map(([key, label]) => <div key={key}><span>{label}</span><b>{confirmed?.counters[key] ?? 0}</b></div>)}
      </div>
      {notice ? <p className="settings-notice" role="status">{notice}</p> : null}
      <footer>
        <small>策略 v{confirmed?.policy_version ?? "-"} · revision {confirmed?.revision ?? "-"} · 当前封禁 {confirmed?.blacklist_size ?? 0}</small>
        <button className="settings-save" type="submit" aria-label="保存 A2S 防御设置" disabled={!confirmed || busy || draftEnabled === confirmed.desired_enabled} aria-busy={busy}>
          {busy ? <RefreshCw /> : <Save />}<span>{busy ? "应用中…" : "保存防御设置"}</span>
        </button>
      </footer>
    </form>
  </section>;
}

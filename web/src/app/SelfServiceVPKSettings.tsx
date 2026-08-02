import { useEffect, useState, type FormEvent } from "react";
import { Globe2, RefreshCw, Save } from "lucide-react";
import { api } from "../api/client";

type Settings = { enabled: boolean; password_set: boolean; auto_delete: boolean; retention_days: number };

export function SelfServiceVPKSettings() {
  const [settings, setSettings] = useState<Settings | null>(null);
  const [password, setPassword] = useState("");
  const [changePassword, setChangePassword] = useState(false);
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState("");
  const [error, setError] = useState("");
  useEffect(() => { void api<Settings>("/api/settings/self-service-vpk").then(setSettings).catch((reason) => setError(reason instanceof Error ? reason.message : String(reason))); }, []);
  async function save(event: FormEvent) {
    event.preventDefault(); if (!settings || busy) return;
    if (!Number.isInteger(settings.retention_days) || settings.retention_days < 1 || settings.retention_days > 365) { setError("自助 VPK 保留天数必须为 1 至 365 的整数"); return; }
    setBusy(true); setError(""); setNotice("");
    try {
      const body: Record<string, unknown> = { enabled: settings.enabled, auto_delete: settings.auto_delete, retention_days: settings.retention_days };
      if (changePassword) body.password = password;
      const saved = await api<Settings>("/api/settings/self-service-vpk", { method: "PUT", body: JSON.stringify(body) });
      setSettings(saved); setPassword(""); setChangePassword(false); setNotice("自助传图设置已保存");
    } catch (reason) { setError(reason instanceof Error ? reason.message : String(reason)); }
    finally { setBusy(false); }
  }
  return <section className="settings-card" aria-labelledby="self-service-vpk-settings-title">
    <div className="settings-card-title"><h3 id="self-service-vpk-settings-title"><Globe2 />自助传图</h3><span className={settings?.enabled ? "configured" : "unconfigured"}>{settings?.enabled ? "已开放" : "未开放"}</span></div>
    <p>开放独立的 <code>/uploadvpk</code> 地图上传入口。密码留空时任何可访问客户端都能查看列表并上传。</p>
    {error ? <p className="error-text" role="alert">{error}</p> : null}
    <form className="settings-fields" onSubmit={save}>
      <label className="settings-toggle"><input type="checkbox" checked={settings?.enabled || false} disabled={!settings || busy} onChange={(event) => setSettings((current) => current ? { ...current, enabled: event.target.checked } : current)} />启用自助传图</label>
      <label className="settings-toggle"><input type="checkbox" checked={settings?.auto_delete || false} disabled={!settings || busy} onChange={(event) => setSettings((current) => current ? { ...current, auto_delete: event.target.checked } : current)} />到期后自动删除</label>
      <label>自助 VPK 保留天数<input aria-label="自助 VPK 保留天数" type="number" min={1} max={365} value={settings?.retention_days ?? 7} disabled={!settings || busy || !settings.auto_delete} onChange={(event) => setSettings((current) => current ? { ...current, retention_days: Number(event.target.value) } : current)} /></label>
      <label className="settings-toggle"><input type="checkbox" checked={changePassword} disabled={!settings || busy} onChange={(event) => setChangePassword(event.target.checked)} />{settings?.password_set ? "重设访问密码" : "设置访问密码"}</label>
      {changePassword ? <label>新访问密码<input aria-label="自助传图访问密码" type="password" value={password} disabled={busy} onChange={(event) => setPassword(event.target.value)} placeholder="留空表示公开访问" /></label> : null}
      {notice ? <p className="settings-notice" role="status">{notice}</p> : null}
      <footer><small>修改密码会立即撤销已有访客授权。</small><button className="settings-save" type="submit" aria-label="保存自助传图设置" disabled={!settings || busy}>{busy ? <RefreshCw /> : <Save />}<span>{busy ? "保存中…" : "保存自助传图设置"}</span></button></footer>
    </form>
  </section>;
}

import { useRef, useState, type FormEvent } from "react";
import { RefreshCw, TerminalSquare, X } from "lucide-react";

export type PackageVersion = {
  id: string;
  filename: string;
  version: string;
  size: number;
  hot_compatible: boolean;
};

export type InstanceConfigValues = {
  name: string;
  game_port: number;
  sourcetv_port: number;
  plugin_ports: number[];
  start_map: string;
  game_mode: string;
  tickrate: number;
  max_players: number;
  extra_args: string;
  package_id: string;
};

export type ConfigurableInstance = InstanceConfigValues & {
  id: string;
  actual_state: string;
  applied_package_id: string;
};

type Props = {
  mode: "create" | "edit";
  instance?: ConfigurableInstance;
  packages: PackageVersion[];
  onClose: () => void;
  onSubmit: (values: InstanceConfigValues) => Promise<void> | void;
};

const createDefaults = (packages: PackageVersion[]): InstanceConfigValues => ({
  name: "",
  game_port: 27015,
  sourcetv_port: 0,
  plugin_ports: [],
  start_map: "c2m1_highway",
  game_mode: "versus",
  tickrate: 100,
  max_players: 32,
  extra_args:
    "-sv_lan 0 -ip 0.0.0.0 +sv_clockcorrection_msecs 25 -timeout 10 +sv_setmax 32 +servercfgfile server.cfg",
  package_id: packages[0]?.id || "",
});

export function buildLaunchPreview(value: InstanceConfigValues) {
  const parts = [
    "./srcds_run",
    "-game",
    "left4dead2",
    "-console",
    "-port",
    String(value.game_port),
    "-tickrate",
    String(value.tickrate),
    "+map",
    value.start_map,
    "+mp_gamemode",
    value.game_mode,
    "-maxplayers",
    String(value.max_players),
  ];
  if (value.sourcetv_port) {
    parts.push(
      "+tv_enable",
      "1",
      "+tv_port",
      String(value.sourcetv_port),
    );
  }
  const extra = value.extra_args.trim();
  return `${parts.join(" ")}${extra ? ` ${extra}` : ""}`;
}

export function InstanceConfigModal({
  mode,
  instance,
  packages,
  onClose,
  onSubmit,
}: Props) {
  const [values, setValues] = useState<InstanceConfigValues>(() =>
    instance
      ? {
          name: instance.name,
          game_port: instance.game_port,
          sourcetv_port: instance.sourcetv_port,
          plugin_ports: instance.plugin_ports,
          start_map: instance.start_map,
          game_mode: instance.game_mode,
          tickrate: instance.tickrate,
          max_players: instance.max_players,
          extra_args: instance.extra_args,
          package_id: instance.package_id,
        }
      : createDefaults(packages),
  );
  const [pluginPorts, setPluginPorts] = useState(() =>
    values.plugin_ports.join(", "),
  );
  const [error, setError] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const submittingRef = useRef(false);
  const setValue = <Key extends keyof InstanceConfigValues>(
    key: Key,
    value: InstanceConfigValues[Key],
  ) => setValues((current) => ({ ...current, [key]: value }));
  const submit = async (event: FormEvent) => {
    event.preventDefault();
    if (submittingRef.current) return;
    submittingRef.current = true;
    setError("");
    setSubmitting(true);
    const parsedPorts = pluginPorts
      .split(",")
      .map((value) => value.trim())
      .filter(Boolean)
      .map(Number);
    try {
      await onSubmit({ ...values, plugin_ports: parsedPorts });
      onClose();
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : String(reason));
    } finally {
      submittingRef.current = false;
      setSubmitting(false);
    }
  };
  const actionLabel =
    mode === "create"
      ? "创建"
      : instance?.actual_state === "uninstalled"
        ? "保存配置"
        : "保存并应用";
  return (
    <div className="modal-wrap instance-config-wrap">
      <form
        className="modal instance-config-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="instance-config-title"
        onSubmit={submit}
      >
        <div className="instance-config-head">
          <div>
            <p className="eyebrow">
              {mode === "create" ? "NEW INSTANCE" : "INSTANCE CONFIG"}
            </p>
            <h2 id="instance-config-title">
              {mode === "create" ? "创建游戏实例" : `配置 ${instance?.name}`}
            </h2>
          </div>
          <button
            type="button"
            className="icon-btn"
            aria-label="关闭实例配置"
            onClick={onClose}
          >
            <X />
          </button>
        </div>
        <div className="instance-config-body">
          <div className="instance-config-fields">
            <label>
              名称
              <input
                value={values.name}
                onChange={(event) => setValue("name", event.target.value)}
                required
              />
            </label>
            <label>
              插件包
              <select
                value={values.package_id}
                onChange={(event) =>
                  setValue("package_id", event.target.value)
                }
                required
              >
                {packages.length ? (
                  <>
                    <option value="" disabled>请选择插件包</option>
                    {packages.map((item) => (
                      <option key={item.id} value={item.id}>
                        {item.filename} · {item.version}
                      </option>
                    ))}
                  </>
                ) : (
                  <option value="">暂无插件包</option>
                )}
              </select>
            </label>
            <label>
              游戏端口
              <input
                type="number"
                min="1024"
                max="65535"
                value={values.game_port}
                onChange={(event) =>
                  setValue("game_port", Number(event.target.value))
                }
                required
              />
            </label>
            <label>
              SourceTV 端口
              <input
                type="number"
                min="0"
                max="65535"
                value={values.sourcetv_port}
                onChange={(event) =>
                  setValue("sourcetv_port", Number(event.target.value))
                }
              />
            </label>
            <label>
              插件端口
              <input
                inputMode="numeric"
                placeholder="27021, 27022"
                value={pluginPorts}
                onChange={(event) => setPluginPorts(event.target.value)}
              />
            </label>
            <label>
              启动地图
              <input
                value={values.start_map}
                onChange={(event) => setValue("start_map", event.target.value)}
                required
              />
            </label>
            <label>
              模式
              <select
                value={values.game_mode}
                onChange={(event) => setValue("game_mode", event.target.value)}
              >
                <option value="coop">合作</option>
                <option value="realism">写实</option>
                <option value="versus">对抗</option>
                <option value="survival">生还者</option>
                <option value="scavenge">清道夫</option>
              </select>
            </label>
            <label>
              Tickrate
              <input
                type="number"
                min="30"
                max="128"
                value={values.tickrate}
                onChange={(event) =>
                  setValue("tickrate", Number(event.target.value))
                }
                required
              />
            </label>
            <label>
              最大玩家
              <input
                type="number"
                min="1"
                max="32"
                value={values.max_players}
                onChange={(event) =>
                  setValue("max_players", Number(event.target.value))
                }
                required
              />
            </label>
            <label className="instance-extra-args">
              额外 SRCDS 启动项
              <textarea
                rows={3}
                value={values.extra_args}
                onChange={(event) => setValue("extra_args", event.target.value)}
              />
            </label>
          </div>
          <section className="command-section">
            <div>
              <TerminalSquare />
              <b>SRCDS / ARGV PREVIEW</b>
            </div>
            <pre aria-label="启动指令预览">{buildLaunchPreview(values)}</pre>
          </section>
        </div>
        {error ? (
          <div className="form-error" role="alert">
            {error}
          </div>
        ) : null}
        <div className="instance-config-actions">
          <button type="button" disabled={submitting} onClick={onClose}>
            取消
          </button>
          <button
            className="create"
            disabled={submitting || !packages.length}
            aria-busy={submitting}
          >
            {submitting ? <RefreshCw /> : null}
            {submitting ? "保存中…" : actionLabel}
          </button>
        </div>
      </form>
    </div>
  );
}

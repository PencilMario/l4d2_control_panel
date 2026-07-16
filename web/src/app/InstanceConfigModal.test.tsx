import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import {
  InstanceConfigModal,
  buildLaunchPreview,
  type ConfigurableInstance,
  type PackageVersion,
} from "./InstanceConfigModal";

const packageA: PackageVersion = {
  id: "package-a",
  filename: "coop-a.zip",
  version: "v1",
  size: 1024,
  hot_compatible: true,
};

const packageB: PackageVersion = {
  id: "package-b",
  filename: "coop-b.zip",
  version: "v2",
  size: 2048,
  hot_compatible: false,
};

const instance: ConfigurableInstance = {
  id: "instance-1",
  name: "深夜战役",
  actual_state: "running",
  game_port: 27015,
  sourcetv_port: 27020,
  plugin_ports: [27021, 27022],
  start_map: "c2m1_highway",
  game_mode: "coop",
  tickrate: 100,
  max_players: 8,
  extra_args: `-strictportbind +hostname "Night Coop"`,
  package_id: packageA.id,
  applied_package_id: packageA.id,
};

describe("InstanceConfigModal", () => {
  it("prevents duplicate submissions while save is pending", () => {
    const submit = vi.fn(() => new Promise<void>(() => undefined));
    render(
      <InstanceConfigModal
        mode="edit"
        instance={instance}
        packages={[packageA, packageB]}
        onClose={vi.fn()}
        onSubmit={submit}
      />,
    );
    const button = screen.getByRole("button", { name: "保存并应用" });

    act(() => {
      button.click();
      button.click();
    });

    expect(submit).toHaveBeenCalledTimes(1);
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-busy", "true");
  });
  it("builds the managed command in runtime order", () => {
    expect(buildLaunchPreview(instance)).toBe(
      `./srcds_run -game left4dead2 -console -port 27015 -tickrate 100 +map c2m1_highway +mp_gamemode coop -maxplayers 8 +tv_enable 1 +tv_port 27020 -strictportbind +hostname "Night Coop"`,
    );
  });

  it("loads the approved SRCDS defaults for new instances", () => {
    render(
      <InstanceConfigModal
        mode="create"
        packages={[packageA]}
        onClose={vi.fn()}
        onSubmit={vi.fn()}
      />,
    );

    expect(screen.getByLabelText("模式")).toHaveValue("versus");
    expect(screen.getByLabelText("Tickrate")).toHaveValue(100);
    expect(screen.getByLabelText("最大玩家")).toHaveValue(32);
    expect(screen.getByLabelText("额外 SRCDS 启动项")).toHaveValue(
      "-sv_lan 0 -ip 0.0.0.0 +sv_clockcorrection_msecs 25 -timeout 10 +sv_setmax 32 +servercfgfile server.cfg",
    );
    expect(screen.getByLabelText("启动指令预览")).toHaveTextContent(
      "./srcds_run -game left4dead2 -console -port 27015 -tickrate 100 +map c2m1_highway +mp_gamemode versus -maxplayers 32 -sv_lan 0 -ip 0.0.0.0 +sv_clockcorrection_msecs 25 -timeout 10 +sv_setmax 32 +servercfgfile server.cfg",
    );
  });

  it("updates the command preview as defaults are edited", async () => {
    render(
      <InstanceConfigModal
        mode="create"
        packages={[packageA]}
        onClose={vi.fn()}
        onSubmit={vi.fn()}
      />,
    );
    await userEvent.clear(screen.getByLabelText("启动地图"));
    await userEvent.type(screen.getByLabelText("启动地图"), "c1m1_hotel");
    await userEvent.type(
      screen.getByLabelText("额外 SRCDS 启动项"),
      `-strictportbind +hostname "Lobby"`,
    );
    expect(screen.getByLabelText("启动指令预览")).toHaveTextContent(
      "+map c1m1_hotel",
    );
    expect(screen.getByLabelText("启动指令预览")).toHaveTextContent(
      `-strictportbind +hostname "Lobby"`,
    );
  });

  it("submits an existing instance package and startup edit", async () => {
    const submit = vi.fn(async () => undefined);
    render(
      <InstanceConfigModal
        mode="edit"
        instance={instance}
        packages={[packageA, packageB]}
        onClose={vi.fn()}
        onSubmit={submit}
      />,
    );
    expect(screen.getByLabelText("游戏端口")).toHaveValue(27015);
    expect(screen.getByLabelText("额外 SRCDS 启动项")).toHaveValue(
      instance.extra_args,
    );
    await userEvent.selectOptions(screen.getByLabelText("插件包"), packageB.id);
    await userEvent.click(
      screen.getByRole("button", { name: "保存并应用" }),
    );
    await waitFor(() =>
      expect(submit).toHaveBeenCalledWith(
        expect.objectContaining({
          package_id: packageB.id,
          extra_args: instance.extra_args,
          plugin_ports: [27021, 27022],
        }),
      ),
    );
  });

  it("shows an explicit empty selection for an instance without a package", () => {
    render(
      <InstanceConfigModal
        mode="edit"
        instance={{ ...instance, package_id: "", applied_package_id: "" }}
        packages={[packageA, packageB]}
        onClose={vi.fn()}
        onSubmit={vi.fn()}
      />,
    );

    expect(screen.getByLabelText("插件包")).toHaveValue("");
    expect(screen.getByLabelText("插件包")).toHaveDisplayValue("请选择插件包");
  });

  it("disables creation when no package is available", () => {
    render(
      <InstanceConfigModal
        mode="create"
        packages={[]}
        onClose={vi.fn()}
        onSubmit={vi.fn()}
      />,
    );
    expect(screen.getByRole("button", { name: "创建" })).toBeDisabled();
    expect(screen.getByLabelText("插件包")).toHaveDisplayValue("暂无插件包");
  });
});

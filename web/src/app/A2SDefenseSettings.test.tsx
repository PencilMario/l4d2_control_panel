import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { A2SDefenseSettings } from "./A2SDefenseSettings";

const disabled = {
  desired_enabled: false,
  effective_enabled: false,
  pending: false,
  compatible: true,
  revision: 3,
  policy_version: 1,
  protected_ports: [27015, 27020],
  counters: { info: 12, player: 4, rules: 2, challenge: 1, other_69: 0, aggregate: 3, blacklist: 5 },
  blacklist_size: 2,
  applied_at: "2026-08-05T02:03:04Z",
  last_error: "",
};

afterEach(() => vi.restoreAllMocks());

it("enables A2S defense and renders effective ports and counters", async () => {
  const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (_input, init) => {
    if (init?.method === "PUT") return Response.json({ ...disabled, desired_enabled: true, effective_enabled: true, revision: 4 });
    return Response.json(disabled);
  });
  render(<A2SDefenseSettings />);
  const toggle = await screen.findByLabelText("启用 A2S 查询防御");
  expect(toggle).not.toBeChecked();
  expect(screen.getByText("27015")).toBeInTheDocument();
  expect(screen.getByText("A2S_INFO").nextSibling).toHaveTextContent("12");
  await userEvent.click(toggle);
  await userEvent.click(screen.getByRole("button", { name: "保存 A2S 防御设置" }));
  expect(fetchMock).toHaveBeenLastCalledWith("/api/settings/a2s-defense", expect.objectContaining({ method: "PUT", body: JSON.stringify({ enabled: true }) }));
  expect(await screen.findByText("防御规则已启用")).toBeInTheDocument();
  expect(toggle).toBeChecked();
});

it("restores the confirmed switch when applying rules fails", async () => {
  vi.spyOn(globalThis, "fetch").mockImplementation(async (_input, init) => {
    if (init?.method === "PUT") return new Response(JSON.stringify({ error: { message: "helper unavailable" } }), { status: 503, headers: { "Content-Type": "application/json" } });
    return Response.json(disabled);
  });
  render(<A2SDefenseSettings />);
  const toggle = await screen.findByLabelText("启用 A2S 查询防御");
  await userEvent.click(toggle);
  await userEvent.click(screen.getByRole("button", { name: "保存 A2S 防御设置" }));
  expect(await screen.findByRole("alert")).toHaveTextContent("helper unavailable");
  expect(toggle).not.toBeChecked();
});

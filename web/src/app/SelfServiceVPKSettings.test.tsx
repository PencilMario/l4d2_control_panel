import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { SelfServiceVPKSettings } from "./SelfServiceVPKSettings";

it("saves public self-service upload settings without returning a plaintext password", async () => {
  const fetchMock = vi.spyOn(globalThis, "fetch").mockImplementation(async (_input, init) => {
    if (init?.method === "PUT") return Response.json({ enabled: true, password_set: false, auto_delete: true, retention_days: 14 });
    return Response.json({ enabled: false, password_set: false, auto_delete: false, retention_days: 7 });
  });
  render(<SelfServiceVPKSettings />);
  await userEvent.click(await screen.findByLabelText("启用自助传图"));
  await userEvent.click(screen.getByLabelText("到期后自动删除"));
  await userEvent.clear(screen.getByLabelText("自助 VPK 保留天数"));
  await userEvent.type(screen.getByLabelText("自助 VPK 保留天数"), "14");
  await userEvent.click(screen.getByRole("button", { name: "保存自助传图设置" }));
  expect(fetchMock).toHaveBeenLastCalledWith("/api/settings/self-service-vpk", expect.objectContaining({ method: "PUT", body: JSON.stringify({ enabled: true, auto_delete: true, retention_days: 14 }) }));
  expect(await screen.findByText("自助传图设置已保存")).toBeInTheDocument();
});

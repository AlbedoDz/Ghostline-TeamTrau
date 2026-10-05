import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Backup } from "./Backup";
import { useGhost } from "../../../../app/store";
import { initI18n } from "../../../../i18n";

const svc = vi.hoisted(() => ({
  ExportSettings: vi.fn(() => Promise.resolve()),
  PreviewImport: vi.fn(),
  ApplyImport: vi.fn(() => Promise.resolve()),
}));
vi.mock("../../../../app/api", () => ({ Service: svc }));

const preview = (extra: Record<string, unknown> = {}) => ({
  token: "tok", path: "C:\\x.ghostline.json",
  preview: {
    appVersion: "0.5.0", createdAt: "2026-10-06T08:00:00Z",
    sections: [
      { name: "settings", new: 0, replaced: 1, errors: [] },
      { name: "rules", new: 3, replaced: 1, errors: [] },
      { name: "customServers", new: 0, replaced: 0, errors: ["udp://1.1.1.1: unencrypted"] },
    ],
    warnings: [{ code: "flag_off", detail: "fakeSni.enabled" }, { code: "trusted_reset", detail: "My list" }],
    sniRules: [],
    ...extra,
  },
});

const status = (s: string) => useGhost.getState().setSnapshot({ status: s, warnings: [], servers: [], blockedSites: [], reasons: [], dpi: { enabled: false } } as any);

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  useGhost.getState().reset();
  status("disconnected");
});

test("export offers every section, all checked", async () => {
  render(<Backup />);
  fireEvent.click(screen.getByRole("button", { name: "xuất cài đặt…" }));
  const boxes = await screen.findAllByRole("checkbox");
  expect(boxes).toHaveLength(5);
  boxes.forEach((b) => expect(b).toBeChecked());
  fireEvent.click(screen.getByRole("checkbox", { name: "server tự thêm" }));
  fireEvent.click(screen.getByRole("button", { name: "xuất" }));
  await waitFor(() => expect(svc.ExportSettings).toHaveBeenCalledWith(["settings", "rules", "dpiBlacklist", "dpiAutoHostlist"]));
});

test("import is disabled while connected, with the reason", () => {
  status("protected");
  render(<Backup />);
  expect(screen.getByRole("button", { name: "nhập cài đặt…" })).toBeDisabled();
  expect(screen.getByText(/Chỉ nhập được khi đã ngắt kết nối/)).toBeInTheDocument();
});

test("preview lists sections, errors and warnings; import applies the choice", async () => {
  svc.PreviewImport.mockResolvedValueOnce(preview());
  render(<Backup />);
  fireEvent.click(screen.getByRole("button", { name: "nhập cài đặt…" }));
  expect(await screen.findByText(/Fake SNI sẽ bị tắt/)).toBeInTheDocument();
  expect(screen.getByText(/My list/)).toBeInTheDocument();
  expect(screen.getByText(/udp:\/\/1\.1\.1\.1/)).toBeInTheDocument();
  expect(screen.getByRole("checkbox", { name: /server tự thêm/ })).toBeDisabled();
  fireEvent.click(screen.getByRole("checkbox", { name: "gộp với dữ liệu hiện có" }));
  fireEvent.click(screen.getByRole("button", { name: "nhập" }));
  await waitFor(() => expect(svc.ApplyImport).toHaveBeenCalledWith("tok", { sections: ["settings", "rules"], merge: true, sniRules: "" }));
  expect(await screen.findByText(/Bấm Kết nối để dùng cấu hình mới/)).toBeInTheDocument();
});

test("rules with sni= need a decision before import", async () => {
  svc.PreviewImport.mockResolvedValueOnce(preview({ sniRules: [{ pattern: "f.com", sni: "cdn.example", enabled: true }] }));
  render(<Backup />);
  fireEvent.click(screen.getByRole("button", { name: "nhập cài đặt…" }));
  expect(await screen.findByText("f.com")).toBeInTheDocument();
  const go = screen.getByRole("button", { name: "nhập" });
  expect(go).toBeDisabled();
  fireEvent.click(screen.getByRole("checkbox", { name: /Tôi hiểu các rule này/ }));
  expect(go).toBeEnabled();
  fireEvent.click(go);
  await waitFor(() => expect(svc.ApplyImport).toHaveBeenCalledWith("tok", { sections: ["settings", "rules"], merge: false, sniRules: "accept" }));
});

test("drop the sni rules instead", async () => {
  svc.PreviewImport.mockResolvedValueOnce(preview({ sniRules: [{ pattern: "f.com", sni: "cdn.example", enabled: true }] }));
  render(<Backup />);
  fireEvent.click(screen.getByRole("button", { name: "nhập cài đặt…" }));
  fireEvent.click(await screen.findByRole("button", { name: "nhập nhưng bỏ các rule này" }));
  await waitFor(() => expect(svc.ApplyImport).toHaveBeenCalledWith("tok", { sections: ["settings", "rules"], merge: false, sniRules: "drop" }));
});

test("IMPORT_EXPIRED asks to choose the file again", async () => {
  svc.PreviewImport.mockResolvedValueOnce(preview());
  svc.ApplyImport.mockRejectedValueOnce(new Error("IMPORT_EXPIRED"));
  render(<Backup />);
  fireEvent.click(screen.getByRole("button", { name: "nhập cài đặt…" }));
  fireEvent.click(await screen.findByRole("button", { name: "nhập" }));
  expect(await screen.findByText(/Chọn lại file/)).toBeInTheDocument();
});

test("a cancelled file dialog does nothing", async () => {
  svc.PreviewImport.mockResolvedValueOnce({ token: "", path: "", preview: null });
  render(<Backup />);
  fireEvent.click(screen.getByRole("button", { name: "nhập cài đặt…" }));
  await waitFor(() => expect(svc.PreviewImport).toHaveBeenCalled());
  expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
});

import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { SimpleView } from "./SimpleView";
import { useGhost } from "../../app/store";
import { initI18n } from "../../i18n";

const svc = vi.hoisted(() => ({
  Connect: vi.fn(() => Promise.resolve()),
  Disconnect: vi.fn(() => Promise.resolve()),
  CancelConnect: vi.fn(() => Promise.resolve()),
  StartAutotune: vi.fn(() => Promise.resolve()),
  RestoreDNSNow: vi.fn(() => Promise.resolve()),
  DismissWarning: vi.fn(() => Promise.resolve()),
  SaveSettings: vi.fn(() => Promise.resolve()),
}));
vi.mock("../../app/api", () => ({ Service: svc }));

const settings = {
  probeSites: ["youtube.com", "discord.com", "telegram.org", "x.com"],
  fragmentDns: { enabled: false, chunks: 5, delayMs: 5 },
  dpi: { enabled: false, preset: "light", customArgs: "", scope: "all" },
} as any;

function snap(over: Record<string, unknown>) {
  return { status: "disconnected", step: 0, warnings: [], servers: [], since: "", latencyMs: 0, queries: 0,
    dpi: { enabled: false, running: false, preset: "light" }, blockedSites: [], ...over } as any;
}

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  useGhost.getState().reset();
  useGhost.getState().setSettings(settings);
});

test("disconnected: clicking power calls Connect", () => {
  useGhost.getState().setSnapshot(snap({}));
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.getByText(/CHƯA BẢO VỆ/)).toBeInTheDocument();
  expect(screen.getByText("bấm để kết nối")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: /CHƯA BẢO VỆ/ }));
  expect(svc.Connect).toHaveBeenCalled();
});

test("connecting: shows steps with current marker and click cancels", () => {
  useGhost.getState().setSnapshot(snap({ status: "connecting", step: 3 }));
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.getByText("chọn máy chủ").closest("[data-step]")).toHaveAttribute("data-step", "done");
  expect(screen.getByText("bật engine").closest("[data-step]")).toHaveAttribute("data-step", "current");
  expect(screen.getByText("đặt DNS").closest("[data-step]")).toHaveAttribute("data-step", "todo");
  fireEvent.click(screen.getByRole("button", { name: /ĐANG KẾT NỐI/ }));
  expect(svc.CancelConnect).toHaveBeenCalled();
});

test("protected with blocked sites shows autotune banner; dismiss hides it", () => {
  useGhost.getState().setSnapshot(snap({ status: "protected", servers: ["Cloudflare", "Quad9"], latencyMs: 24, blockedSites: ["youtube.com", "x.com"], since: new Date().toISOString() }));
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.getByText(/ĐÃ BẢO VỆ/)).toBeInTheDocument();
  expect(screen.getByText("Cloudflare +1")).toBeInTheDocument();
  expect(screen.getByText(/2\/4 trang mẫu vẫn bị chặn/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "TỰ DÒ VƯỢT DPI" }));
  expect(svc.StartAutotune).toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "bỏ qua" }));
  expect(screen.queryByText(/trang mẫu vẫn bị chặn/)).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: /ĐÃ BẢO VỆ/ }));
  expect(svc.Disconnect).toHaveBeenCalled();
});

test("error NO_SERVERS shows unchanged-DNS reassurance and both actions", async () => {
  useGhost.getState().setSnapshot(snap({ status: "error", error: { code: "NO_SERVERS", params: { checked: 142, elapsed: 20 } } }));
  const onOpenLogs = vi.fn();
  render(<SimpleView onOpenLogs={onOpenLogs} />);
  expect(screen.getByText("DNS của máy vẫn như cũ, không có gì bị thay đổi")).toBeInTheDocument();
  expect(screen.getByText(/142 đã thử trong 20s/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "BẬT FRAGMENT DNS" }));
  await Promise.resolve();
  expect(svc.SaveSettings).toHaveBeenCalledWith(expect.objectContaining({ fragmentDns: expect.objectContaining({ enabled: true }) }));
  fireEvent.click(screen.getByRole("button", { name: "thử lại" }));
  expect(svc.Connect).toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "mở nhật ký ›" }));
  expect(onOpenLogs).toHaveBeenCalled();
});

test("RESTORE_FAILED warning shows restore button and cannot be dismissed", () => {
  useGhost.getState().setSnapshot(snap({ warnings: [{ code: "RESTORE_FAILED", params: { adapter: "Wi-Fi" } }] }));
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.getByText(/Wi-Fi/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: /KHÔI PHỤC DNS NGAY/ }));
  expect(svc.RestoreDNSNow).toHaveBeenCalled();
  expect(screen.queryByRole("button", { name: "đã hiểu" })).toBeNull();
});

test("SETTINGS_RESET warning can be acknowledged", () => {
  useGhost.getState().setSnapshot(snap({ warnings: [{ code: "SETTINGS_RESET" }] }));
  render(<SimpleView onOpenLogs={() => {}} />);
  fireEvent.click(screen.getByRole("button", { name: "đã hiểu" }));
  expect(svc.DismissWarning).toHaveBeenCalledWith("SETTINGS_RESET");
});

test("English locale renders English strings", async () => {
  await initI18n("en");
  useGhost.getState().setSnapshot(snap({}));
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.getByText(/UNPROTECTED/)).toBeInTheDocument();
  expect(screen.getByText("tap to connect")).toBeInTheDocument();
  await initI18n("vi");
});

test("RESTORE_FAILED error never claims DNS is unchanged", () => { // review C1
  useGhost.getState().setSnapshot(snap({ status: "error", error: { code: "RESTORE_FAILED", params: { adapter: "Wi-Fi" } } }));
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.queryByText("DNS của máy vẫn như cũ, không có gì bị thay đổi")).toBeNull();
});

test("a failed restore from the warning banner is shown", async () => { // review minor
  svc.RestoreDNSNow.mockRejectedValueOnce(new Error("netsh failed"));
  useGhost.getState().setSnapshot(snap({ warnings: [{ code: "RESTORE_FAILED", params: { adapter: "Wi-Fi" } }] }));
  render(<SimpleView onOpenLogs={() => {}} />);
  fireEvent.click(screen.getByRole("button", { name: /KHÔI PHỤC DNS NGAY/ }));
  expect(await screen.findByText(/netsh failed/)).toBeInTheDocument();
});

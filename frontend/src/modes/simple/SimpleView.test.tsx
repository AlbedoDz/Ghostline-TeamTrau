import { beforeAll, beforeEach, describe, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen } from "@testing-library/react";
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
  DPIStrategies: vi.fn(() => Promise.resolve([{ id: "z-split", name: { vi: "Nhẹ", en: "Light" } }])),
}));
vi.mock("../../app/api", () => ({ Service: svc }));
const browser = vi.hoisted(() => ({ OpenURL: vi.fn(() => Promise.resolve()) }));
vi.mock("@wailsio/runtime", () => ({ Browser: browser }));

const settings = {
  probeSites: ["youtube.com", "discord.com", "x.com"],
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
  expect(screen.getByText(/2\/3 trang mẫu vẫn bị chặn/)).toBeInTheDocument();
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

test("shows a newer release and opens it", () => {
  useGhost.getState().setSnapshot(snap({}));
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.queryByText(/có bản mới/)).toBeNull();
  act(() => useGhost.getState().setUpdate({ tag: "v0.1.1", url: "https://example/v0.1.1" }));
  fireEvent.click(screen.getByRole("button", { name: /có bản mới v0\.1\.1/ }));
  expect(browser.OpenURL).toHaveBeenCalledWith("https://example/v0.1.1");
});

test("a release remembered from an earlier check shows after restart", () => {
  useGhost.getState().setSnapshot(snap({}));
  useGhost.getState().setInfo({ version: "0.1.0", portable: false, updateTag: "v0.1.1", updateUrl: "https://example/v0.1.1" } as any);
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.getByRole("button", { name: /có bản mới v0\.1\.1/ })).toBeInTheDocument();
});

test("a running zapret2 strategy shows by name, not by id", async () => {
  useGhost.getState().setSnapshot(snap({ status: "protected", dpi: { enabled: true, running: true, engine: "zapret2", preset: "z-split" } }));
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(await screen.findByText("Nhẹ ✓")).toBeInTheDocument();
  expect(svc.DPIStrategies).toHaveBeenCalledWith("zapret2");
});

test("a GoodbyeDPI preset shows its translated name", async () => {
  useGhost.getState().setSnapshot(snap({ status: "protected", dpi: { enabled: true, running: true, engine: "goodbyedpi", preset: "mode3" } }));
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(await screen.findByText("Mode 3 ✓")).toBeInTheDocument();
});

test("auto-tune progress and result name the strategy", async () => {
  useGhost.getState().setSnapshot(snap({ status: "protected", dpi: { enabled: true, running: true, engine: "zapret2", preset: "z-split" } }));
  useGhost.getState().setAutotune({ running: true, engine: "zapret2", preset: "z-split", index: 1, total: 4 } as any);
  const { rerender } = render(<SimpleView onOpenLogs={() => {}} />);
  expect(await screen.findByText(/đang dò: Nhẹ \(1\/4\)/)).toBeInTheDocument();
  useGhost.getState().setAutotune({ running: false, engine: "zapret2", preset: "z-split", index: 0, total: 0 } as any);
  rerender(<SimpleView onOpenLogs={() => {}} />);
  expect(await screen.findByText("✓ tự dò đã chọn Nhẹ (zapret2)")).toBeInTheDocument();
});

describe("disconnect while LAN devices use this PC's DNS", () => {
  const lanSnap = (n: number) => {
    useGhost.getState().setSnapshot(snap({ status: "protected", servers: ["Cloudflare"], since: new Date().toISOString(),
      dnsServer: { running: true, addrs: ["192.168.1.8:443"] } }));
    useGhost.getState().setDnsStats({ queries: 10, clients10m: n, clientIps: [] } as any);
  };

  test("asks first and stays connected on cancel", () => {
    lanSnap(2);
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(false);
    render(<SimpleView onOpenLogs={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: /ĐÃ BẢO VỆ/ }));
    expect(confirm.mock.calls[0][0]).toMatch(/2 thiết bị/);
    expect(svc.Disconnect).not.toHaveBeenCalled();
    confirm.mockRestore();
  });

  test("disconnects after confirming", () => {
    lanSnap(1);
    const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
    render(<SimpleView onOpenLogs={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: /ĐÃ BẢO VỆ/ }));
    expect(svc.Disconnect).toHaveBeenCalled();
    confirm.mockRestore();
  });

  test("no question when no LAN device used it", () => {
    lanSnap(0);
    const confirm = vi.spyOn(window, "confirm");
    render(<SimpleView onOpenLogs={() => {}} />);
    fireEvent.click(screen.getByRole("button", { name: /ĐÃ BẢO VỆ/ }));
    expect(confirm).not.toHaveBeenCalled();
    expect(svc.Disconnect).toHaveBeenCalled();
    confirm.mockRestore();
  });
});

test("connecting: the server step shows how many servers were checked", () => {
  useGhost.getState().setSnapshot(snap({ status: "connecting", step: 2, pickDone: 120, pickTotal: 900 }));
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.getByText("chọn máy chủ 120/900").closest("[data-step]")).toHaveAttribute("data-step", "current");
});

test("connecting: a first scan says it is slow only this time", () => {
  useGhost.getState().setSnapshot(snap({ status: "connecting", step: 2, pickDone: 170, pickTotal: 906 }));
  const { rerender } = render(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.getByText(/chọn máy chủ 170\/906/)).toBeInTheDocument();
  expect(screen.getByTestId("step-hint")).toHaveTextContent("lần đầu trên mạng này sẽ chậm (đo mọi máy chủ), lần sau chỉ vài giây");
  act(() => useGhost.getState().setSnapshot(snap({ status: "connecting", step: 2 })));
  rerender(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.queryByTestId("step-hint"), "from the ranking: nothing to explain").toBeNull();
});

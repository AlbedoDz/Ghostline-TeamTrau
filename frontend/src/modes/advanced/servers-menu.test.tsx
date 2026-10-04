import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { Servers } from "./pages/Servers";
import { ConnectError } from "../../components/ConnectError";
import { useGhost } from "../../app/store";
import { initI18n } from "../../i18n";

const rows = [
  { server: { id: "cf", name: "Cloudflare", protocol: "doh", address: "https://cloudflare-dns.com/dns-query", ips: ["1.1.1.1"], tags: ["no-filter"], source: "builtin" }, result: { serverId: "cf", ok: true, latency: 18e6 }, inUse: true, pinned: false },
  { server: { id: "q9", name: "Quad9", protocol: "dot", address: "tls://dns.quad9.net", tags: ["no-filter"], source: "builtin" }, result: { serverId: "q9", ok: true, latency: 24e6 }, inUse: false, pinned: true },
  { server: { id: "my", name: "My DoH", protocol: "doh", address: "https://my.example/dns-query", tags: [], source: "custom" }, inUse: false, pinned: false },
];

const svc = vi.hoisted(() => ({
  ListServers: vi.fn(),
  SetPinned: vi.fn(() => Promise.resolve()),
  SetPinnedMany: vi.fn(() => Promise.resolve()),
  UseOnlyServer: vi.fn(() => Promise.resolve()),
  CheckServer: vi.fn(),
  RemoveCustomServer: vi.fn(() => Promise.resolve()),
  SaveSettings: vi.fn(() => Promise.resolve()),
  GetSettings: vi.fn(),
  Connect: vi.fn(() => Promise.resolve()),
  Disconnect: vi.fn(() => Promise.resolve()),
  ScanAll: vi.fn(() => Promise.resolve()),
  CancelScan: vi.fn(() => Promise.resolve()),
}));
vi.mock("../../app/api", () => ({ Service: svc }));

const settings = { pinnedOnly: false, pinned: ["q9"], includeTags: ["no-filter"], fragmentDns: { enabled: false } };
const clipboard = { writeText: vi.fn(() => Promise.resolve()) };

beforeAll(() => {
  initI18n("vi");
  Object.assign(navigator, { clipboard });
});
beforeEach(() => {
  vi.clearAllMocks();
  svc.ListServers.mockResolvedValue(structuredClone(rows));
  svc.GetSettings.mockResolvedValue(structuredClone(settings));
  useGhost.getState().reset();
  useGhost.getState().setSettings(structuredClone(settings) as any);
});

const rowOf = async (name: string) => (await screen.findByText(name)).closest("tr")!;
const openMenu = async (name: string) => {
  fireEvent.contextMenu(await rowOf(name), { clientX: 100, clientY: 120 });
  return screen.getByRole("menu");
};

test("right-click shows the row actions; copy IP only with IPs, delete only for custom", async () => {
  render(<Servers />);
  let menu = await openMenu("Cloudflare");
  const items = within(menu).getAllByRole("menuitem").map((i) => i.textContent);
  expect(items).toEqual(["★ ghim", "⚡ chỉ dùng máy chủ này", "⟳ kiểm tra lại", "⧉ sao chép địa chỉ", "⧉ sao chép IP"]);
  fireEvent.click(within(menu).getByRole("menuitem", { name: "⧉ sao chép địa chỉ" }));
  expect(clipboard.writeText).toHaveBeenCalledWith("https://cloudflare-dns.com/dns-query");
  expect(screen.queryByRole("menu")).not.toBeInTheDocument();

  menu = await openMenu("Quad9");
  expect(within(menu).getByRole("menuitem", { name: "☆ bỏ ghim" })).toBeInTheDocument();
  expect(within(menu).queryByRole("menuitem", { name: "⧉ sao chép IP" })).not.toBeInTheDocument();
  fireEvent.keyDown(menu, { key: "Escape" });
  expect(screen.queryByRole("menu")).not.toBeInTheDocument();

  menu = await openMenu("My DoH");
  fireEvent.click(within(menu).getByRole("menuitem", { name: "✕ xoá" }));
  await waitFor(() => expect(svc.RemoveCustomServer).toHaveBeenCalledWith("my"));
});

test("menu pin and re-check update the row", async () => {
  svc.CheckServer.mockResolvedValueOnce({ ...rows[2], result: { serverId: "my", ok: true, latency: 7e6 } });
  render(<Servers />);
  fireEvent.click(within(await openMenu("Cloudflare")).getByRole("menuitem", { name: "★ ghim" }));
  await waitFor(() => expect(svc.SetPinned).toHaveBeenCalledWith("cf", true));
  fireEvent.click(within(await openMenu("My DoH")).getByRole("menuitem", { name: "⟳ kiểm tra lại" }));
  await waitFor(() => expect(svc.CheckServer).toHaveBeenCalledWith("my"));
  expect(await within(await rowOf("My DoH")).findByText("7 ms")).toBeInTheDocument();
});

test("use only this server asks when other pins exist, then reconnects if connected", async () => {
  useGhost.getState().setSnapshot({ status: "protected", warnings: [], servers: [], dpi: {} } as any);
  const confirm = vi.spyOn(window, "confirm").mockReturnValueOnce(false).mockReturnValueOnce(true);
  render(<Servers />);
  fireEvent.click(within(await openMenu("Cloudflare")).getByRole("menuitem", { name: "⚡ chỉ dùng máy chủ này" }));
  expect(confirm).toHaveBeenCalledTimes(1);
  expect(svc.UseOnlyServer).not.toHaveBeenCalled();
  fireEvent.click(within(await openMenu("Cloudflare")).getByRole("menuitem", { name: "⚡ chỉ dùng máy chủ này" }));
  await waitFor(() => expect(svc.UseOnlyServer).toHaveBeenCalledWith("cf"));
  await waitFor(() => expect(svc.Connect).toHaveBeenCalled());
  expect(svc.Disconnect).toHaveBeenCalled();
});

test("keyboard: Shift+F10 on a row opens the menu, arrows and Enter run an item", async () => {
  render(<Servers />);
  const row = await rowOf("Cloudflare");
  row.focus();
  fireEvent.keyDown(row, { key: "F10", shiftKey: true });
  const menu = screen.getByRole("menu");
  fireEvent.keyDown(menu, { key: "ArrowDown" }); // ★ ghim → ⚡ chỉ dùng…
  fireEvent.keyDown(menu, { key: "ArrowDown" }); // → ⟳ kiểm tra lại
  svc.CheckServer.mockResolvedValueOnce(rows[0]);
  fireEvent.keyDown(menu, { key: "Enter" });
  await waitFor(() => expect(svc.CheckServer).toHaveBeenCalledWith("cf"));
});

test("pinned-only switch is disabled with nothing pinned; unpin all turns it off", async () => {
  svc.ListServers.mockResolvedValue(rows.map((r) => ({ ...r, pinned: false })));
  render(<Servers />);
  await rowOf("Cloudflare");
  const sw = screen.getByRole("switch", { name: "chỉ dùng máy chủ đã ghim" });
  expect(sw).toBeDisabled();
  const hint = "ⓘ Chưa ghim máy chủ nào. Bấm ☆ hoặc double-click một dòng để ghim; cần có ít nhất một máy chủ ghim mới bật được \"chỉ dùng máy chủ đã ghim\".";
  expect(screen.queryByText(hint)).not.toBeInTheDocument(); // tooltip only, no extra line
  expect(sw.closest("[title]")?.getAttribute("title")).toBe(hint);
});

test("unpin all with pinned-only on reloads settings (Go turns it off)", async () => {
  useGhost.getState().setSettings({ ...structuredClone(settings), pinnedOnly: true } as any);
  render(<Servers />);
  await rowOf("Quad9");
  fireEvent.click(screen.getByRole("button", { name: "bỏ ghim tất cả" }));
  await waitFor(() => expect(svc.GetSettings).toHaveBeenCalled());
  await waitFor(() => expect(useGhost.getState().settings?.pinnedOnly).toBe(false));
});

test("NO_PINNED_SERVERS error offers opening Servers and turning pinned-only off", async () => {
  useGhost.getState().setSettings({ ...structuredClone(settings), pinnedOnly: true, mode: "simple" } as any);
  useGhost.getState().setSnapshot({ status: "error", error: { code: "NO_PINNED_SERVERS", params: { checked: 0 } }, warnings: [], dpi: {} } as any);
  const onOpenServers = vi.fn();
  render(<ConnectError onOpenServers={onOpenServers} onOpenLogs={() => {}} />);
  expect(screen.getByText(/chưa có máy chủ ghim nào dùng được/i)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "mở trang máy chủ" }));
  expect(onOpenServers).toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "tắt chỉ dùng máy chủ ghim" }));
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalled());
  expect((svc.SaveSettings.mock.calls[0] as any[])[0].pinnedOnly).toBe(false);
  await waitFor(() => expect(svc.Connect).toHaveBeenCalled());
});

test("connect errors are shown in Advanced mode too", async () => {
  act(() => useGhost.getState().setSnapshot({ status: "error", error: { code: "NO_SERVERS", params: { checked: 3, elapsed: 2 } }, warnings: [], dpi: {} } as any));
  render(<ConnectError onOpenServers={() => {}} onOpenLogs={() => {}} />);
  expect(screen.getByRole("alert")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "thử lại" })).toBeInTheDocument();
});

test("double-click a row toggles its pin", async () => {
  render(<Servers />);
  fireEvent.doubleClick(await rowOf("Cloudflare"));
  await waitFor(() => expect(svc.SetPinned).toHaveBeenCalledWith("cf", true));
  fireEvent.doubleClick(await rowOf("Quad9"));
  await waitFor(() => expect(svc.SetPinned).toHaveBeenCalledWith("q9", false));
});

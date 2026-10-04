import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { Servers } from "./pages/Servers";
import { Overview } from "./pages/Overview";
import { AdvancedView } from "./AdvancedView";
import { useGhost } from "../../app/store";
import { initI18n } from "../../i18n";

const rows = [
  { server: { id: "cf", name: "Cloudflare", protocol: "doh", address: "https://c", tags: ["no-filter"], source: "builtin" }, result: { serverId: "cf", ok: true, latency: 18e6 }, inUse: true, pinned: false },
  { server: { id: "q9", name: "Quad9", protocol: "dot", address: "tls://q", tags: ["no-filter"], source: "builtin" }, result: { serverId: "q9", ok: true, latency: 24e6 }, inUse: false, pinned: true },
  { server: { id: "gg", name: "Google", protocol: "doh", address: "https://g", tags: ["no-filter"], source: "builtin" }, result: { serverId: "gg", ok: false, reason: "timeout", latency: 0 }, inUse: false, pinned: false },
  { server: { id: "ad", name: "AdGuard", protocol: "dnscrypt", address: "sdns://x", tags: ["adblock"], source: "dnscrypt" }, inUse: false, pinned: false },
];

const svc = vi.hoisted(() => ({
  ListServers: vi.fn(),
  SetPinned: vi.fn(() => Promise.resolve()),
  SetPinnedMany: vi.fn(() => Promise.resolve()),
  GetSettings: vi.fn(() => Promise.resolve(null)),
  ScanAll: vi.fn(() => Promise.resolve()),
  CancelScan: vi.fn(() => Promise.resolve()),
  AddServers: vi.fn(() => Promise.resolve([1, ["udp://1.1.1.1"]])),
  RemoveCustomServer: vi.fn(() => Promise.resolve()),
  SaveSettings: vi.fn(() => Promise.resolve()),
  Connect: vi.fn(() => Promise.resolve()),
  Disconnect: vi.fn(() => Promise.resolve()),
  CancelConnect: vi.fn(() => Promise.resolve()),
}));
vi.mock("../../app/api", () => ({ Service: svc }));

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  svc.ListServers.mockResolvedValue(rows);
  useGhost.getState().reset();
  useGhost.getState().setSettings({ pinnedOnly: false, includeTags: ["no-filter"] } as any);
});

const names = () => screen.getAllByRole("row").slice(1).map((r) => within(r).getAllByRole("cell")[1].querySelector("[data-name]")?.textContent);

test("servers table filters by protocol chip and only-ok", async () => {
  render(<Servers />);
  await waitFor(() => expect(names()).toHaveLength(4));
  expect(names()[0]).toBe("Quad9"); // pinned first, then latency asc, unchecked last
  expect(names()[1]).toBe("Cloudflare");
  fireEvent.click(screen.getByRole("button", { name: "dot" }));
  expect(names()).not.toContain("Quad9");
  fireEvent.click(screen.getByRole("button", { name: "chỉ đạt" }));
  expect(names()).toEqual(["Cloudflare"]);
});

test("pin toggle calls SetPinned", async () => {
  render(<Servers />);
  await waitFor(() => expect(names()).toHaveLength(4));
  fireEvent.click(screen.getByRole("button", { name: "ghim Cloudflare" }));
  expect(svc.SetPinned).toHaveBeenCalledWith("cf", true);
});

test("scan button shows progress from scan:progress events and cancels on second click", async () => {
  render(<Servers />);
  fireEvent.click(screen.getByRole("button", { name: /quét toàn bộ/ }));
  expect(svc.ScanAll).toHaveBeenCalled();
  act(() => useGhost.getState().setScan({ done: 3, total: 10, running: true } as any));
  const btn = screen.getByRole("button", { name: /đang quét 3\/10/ });
  fireEvent.click(btn);
  expect(svc.CancelScan).toHaveBeenCalled();
});

test("add dialog reports added count and rejected lines", async () => {
  render(<Servers />);
  fireEvent.click(screen.getByRole("button", { name: "+ thêm" }));
  fireEvent.change(screen.getByRole("textbox"), { target: { value: "https://a/dns-query\nudp://1.1.1.1" } });
  fireEvent.click(screen.getByRole("button", { name: "lưu" }));
  await screen.findByText("đã thêm 1 máy chủ");
  expect(screen.getByText(/bị từ chối: udp:\/\/1\.1\.1\.1/)).toBeInTheDocument();
  expect(svc.AddServers).toHaveBeenCalledWith("https://a/dns-query\nudp://1.1.1.1");
});

test("overview sparkline receives latency points", () => {
  useGhost.getState().setSnapshot({ status: "protected", servers: ["Cloudflare"], warnings: [], blockedSites: [], dpi: {} } as any);
  for (const v of [10, 20, 30]) useGhost.getState().pushStats({ queries: 5, latencyMs: v });
  const { container } = render(<Overview />);
  expect(container.querySelector("polyline")!.getAttribute("points")!.split(" ")).toHaveLength(3);
  expect(screen.getByText("Cloudflare")).toBeInTheDocument();
});

test("advanced view switches pages from the sidebar", async () => {
  useGhost.getState().setSnapshot({ status: "disconnected", servers: [], warnings: [], blockedSites: [], dpi: {} } as any);
  render(<AdvancedView />);
  fireEvent.click(screen.getByRole("button", { name: "máy chủ" }));
  expect(useGhost.getState().page).toBe("servers");
  await screen.findByText(/MÁY CHỦ/);
});

test("duplicate server names are told apart by host or IP", async () => {
  svc.ListServers.mockResolvedValue([
    { server: { id: "a1", name: "AdGuard (unfiltered)", protocol: "doh", address: "https://unfiltered.adguard-dns.com/dns-query", source: "builtin", tags: ["no-filter"] }, inUse: false, pinned: false },
    { server: { id: "a2", name: "AdGuard (unfiltered)", protocol: "dot", address: "tls://unfiltered.adguard-dns.com", source: "builtin", tags: ["no-filter"] }, inUse: false, pinned: false },
    { server: { id: "dnscrypt:a-and-a", name: "a-and-a", protocol: "doh", address: "sdns://x", ips: ["217.169.20.22"], source: "dnscrypt", tags: ["no-filter"] }, inUse: false, pinned: false },
  ]);
  render(<Servers />);
  await waitFor(() => expect(names()).toHaveLength(3));
  expect(names()).toContain("A And A"); // DNSCrypt ids are made readable
  expect(screen.getAllByText("unfiltered.adguard-dns.com")).toHaveLength(2);
  expect(screen.getByText("217.169.20.22")).toBeInTheDocument();
  expect(screen.getAllByText("DOH").length).toBeGreaterThan(0);
});

test("sidebar status explains its numbers and offers a clear button", () => {
  useGhost.getState().setSnapshot({ status: "protected", servers: ["a", "b", "c", "d", "e"], latencyMs: 26, warnings: [], blockedSites: [], dpi: {} } as any);
  render(<AdvancedView />);
  expect(screen.getByText(/26 ms · 5 máy chủ/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "⏻ NGẮT KẾT NỐI" }));
  expect(svc.Disconnect).toHaveBeenCalled();
});

test("sidebar button connects when disconnected", () => {
  useGhost.getState().setSnapshot({ status: "disconnected", servers: [], warnings: [], blockedSites: [], dpi: {} } as any);
  render(<AdvancedView />);
  fireEvent.click(screen.getByRole("button", { name: "⏻ KẾT NỐI" }));
  expect(svc.Connect).toHaveBeenCalled();
});

test("search filters servers by name, address, IP and tag, and can be cleared", async () => {
  const withIPs = rows.map((r) => (r.server.id === "q9" ? { ...r, server: { ...r.server, ips: ["9.9.9.9"] } } : r));
  svc.ListServers.mockResolvedValue(withIPs);
  render(<Servers />);
  await waitFor(() => expect(names()).toHaveLength(4));
  const box = screen.getByRole("searchbox", { name: "tìm máy chủ" });

  fireEvent.change(box, { target: { value: "GOOG" } }); // case-insensitive name
  expect(names()).toEqual(["Google"]);
  expect(screen.getByText(/khớp 1$/)).toBeInTheDocument();
  fireEvent.change(box, { target: { value: "doh no-filter" } }); // every word must match
  expect(names()).toEqual(["Cloudflare", "Google"]);
  fireEvent.change(box, { target: { value: "9.9.9.9" } }); // IP
  expect(names()).toEqual(["Quad9"]);
  fireEvent.change(box, { target: { value: "sdns://" } }); // address
  expect(names()).toEqual(["AdGuard"]);
  fireEvent.change(box, { target: { value: "adblock" } }); // tag
  expect(names()).toEqual(["AdGuard"]);

  // Combines with the chips.
  fireEvent.change(box, { target: { value: "o" } });
  fireEvent.click(screen.getByRole("button", { name: "chỉ đạt" }));
  expect(names()).toEqual(["Quad9", "Cloudflare"]); // pinned first

  fireEvent.change(box, { target: { value: "nothing-matches" } });
  expect(screen.getByText("không có máy chủ nào khớp")).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", { name: "xoá tìm kiếm" }));
  expect((box as HTMLInputElement).value).toBe("");
  expect(names()).toEqual(["Quad9", "Cloudflare"]); // pinned first
});

test("pinned servers: chip filter, pinned-only switch at the top, bulk pin and unpin", async () => {
  render(<Servers />);
  await waitFor(() => expect(names()).toHaveLength(4));
  // The pinned-only switch sits above the table now.
  const sw = screen.getByRole("switch", { name: "chỉ dùng máy chủ đã ghim" });
  expect(sw.compareDocumentPosition(screen.getByRole("table")) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();

  expect(screen.getByText("máy chủ đã ghim được ưu tiên khi kết nối")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "★ đã ghim (1)" }));
  expect(names()).toEqual(["Quad9"]);
  fireEvent.click(screen.getByRole("button", { name: "★ đã ghim (1)" }));
  expect(names()).toHaveLength(4);

  fireEvent.change(screen.getByRole("searchbox", { name: "tìm máy chủ" }), { target: { value: "doh" } });
  fireEvent.click(screen.getByRole("button", { name: "★ ghim tất cả kết quả (2)" }));
  await waitFor(() => expect(svc.SetPinnedMany).toHaveBeenCalledWith(["cf", "gg"], true));

  fireEvent.click(screen.getByRole("button", { name: "bỏ ghim tất cả" }));
  await waitFor(() => expect(svc.SetPinnedMany).toHaveBeenCalledWith(["q9"], false));
});

test("changing pins while connected offers a reconnect", async () => {
  useGhost.getState().setSnapshot({ status: "protected", warnings: [], servers: ["Cloudflare"], dpi: {} } as any);
  render(<Servers />);
  await waitFor(() => expect(names()).toHaveLength(4));
  expect(screen.queryByText(/đã đổi máy chủ ghim/)).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "ghim Google" }));
  fireEvent.click(await screen.findByRole("button", { name: "kết nối lại để áp dụng" }));
  await waitFor(() => expect(svc.Connect).toHaveBeenCalled());
  expect(svc.Disconnect.mock.invocationCallOrder[0]).toBeLessThan(svc.Connect.mock.invocationCallOrder[0]);
});

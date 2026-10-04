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

const names = () => screen.getAllByRole("row").slice(1).map((r) => within(r).getAllByRole("cell")[1].textContent);

test("servers table filters by protocol chip and only-ok", async () => {
  render(<Servers />);
  await waitFor(() => expect(names()).toHaveLength(4));
  expect(names()[0]).toBe("Cloudflare"); // sorted by latency asc, unchecked last
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

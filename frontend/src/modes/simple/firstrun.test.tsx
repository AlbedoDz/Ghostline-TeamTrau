import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import { SimpleView } from "./SimpleView";
import { useGhost } from "../../app/store";
import { initI18n } from "../../i18n";

const svc = vi.hoisted(() => ({
  ProbeNow: vi.fn(),
  StartAutotune: vi.fn(() => Promise.resolve()),
  MarkNetworkChecked: vi.fn(() => Promise.resolve()),
  SaveSettings: vi.fn(() => Promise.resolve()),
  DPIStrategies: vi.fn(() => Promise.resolve([])),
}));
vi.mock("../../app/api", () => ({ Service: svc }));
vi.mock("@wailsio/runtime", () => ({ Browser: { OpenURL: vi.fn() } }));

const settings = (checked: boolean) =>
  ({
    probeSites: ["youtube.com", "discord.com", "x.com"],
    fragmentDns: { enabled: false, chunks: 5, delayMs: 5 },
    dpi: { enabled: false, engine: "zapret2", preset: "light", scope: "all" },
    proxy: { enabled: false, systemProxy: false },
    fakeSni: { enabled: false },
    simple: { checked },
  }) as any;

const snap = (status: string, probed = false, blockedSites: string[] = []) =>
  ({ status, step: 0, warnings: [], servers: ["Cloudflare"], since: new Date().toISOString(), latencyMs: 20, queries: 0,
    blockedSites, probed, reasons: [], dpi: { enabled: false, running: false } }) as any;

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  useGhost.getState().reset();
});

test("first connect: waits for Go's site check, then auto-tunes the blocked sites as a step", async () => {
  useGhost.getState().setSettings(settings(false));
  useGhost.getState().setSnapshot(snap("protected"));
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.getByText(/kiểm tra trang mẫu/)).toBeInTheDocument();
  expect(svc.StartAutotune, "DPI bypass may still be starting").not.toHaveBeenCalled();

  act(() => useGhost.getState().setSnapshot(snap("protected", true, ["youtube.com"])));
  await waitFor(() => expect(svc.StartAutotune).toHaveBeenCalledTimes(1));
  expect(svc.ProbeNow, "no probing of its own").not.toHaveBeenCalled();

  act(() => useGhost.getState().setAutotune({ running: true, engine: "zapret2", preset: "z-split", index: 2, total: 4 } as any));
  expect(screen.getByText(/tự dò vượt DPI: thử cách 2\/4/)).toBeInTheDocument();
  expect(screen.getByText("đặt DNS").closest("[data-step]")).toHaveAttribute("data-step", "done");
  expect(screen.queryByText(/đang dò:/), "no separate banner").toBeNull();
  expect(svc.MarkNetworkChecked).not.toHaveBeenCalled();

  act(() => useGhost.getState().setSnapshot(snap("protected", true, [])));
  act(() => useGhost.getState().setAutotune({ running: false, engine: "zapret2", preset: "z-split", index: 0, total: 0 } as any));
  await waitFor(() => expect(svc.MarkNetworkChecked).toHaveBeenCalled());
  expect(useGhost.getState().settings?.simple?.checked).toBe(true);
  expect(screen.queryByText(/tự dò vượt DPI: thử cách/)).toBeNull();
});

test("first connect: nothing blocked, nothing tuned", async () => {
  useGhost.getState().setSettings(settings(false));
  useGhost.getState().setSnapshot(snap("protected", true));
  render(<SimpleView onOpenLogs={() => {}} />);
  await waitFor(() => expect(svc.MarkNetworkChecked).toHaveBeenCalled());
  expect(svc.StartAutotune).not.toHaveBeenCalled();
});

test("only on the first run, and only once connected", async () => {
  useGhost.getState().setSettings(settings(true));
  useGhost.getState().setSnapshot(snap("protected", true, ["youtube.com"]));
  const { unmount } = render(<SimpleView onOpenLogs={() => {}} />);
  unmount();
  useGhost.getState().setSettings(settings(false));
  useGhost.getState().setSnapshot(snap("disconnected"));
  render(<SimpleView onOpenLogs={() => {}} />);
  await new Promise((r) => setTimeout(r, 20));
  expect(svc.StartAutotune).not.toHaveBeenCalled();
});

test("disconnecting mid-way leaves it for the next connect", async () => {
  useGhost.getState().setSettings(settings(false));
  useGhost.getState().setSnapshot(snap("protected", true, ["youtube.com"]));
  render(<SimpleView onOpenLogs={() => {}} />);
  await waitFor(() => expect(svc.StartAutotune).toHaveBeenCalledTimes(1));
  act(() => useGhost.getState().setAutotune({ running: true, index: 3, total: 4 } as any));
  act(() => useGhost.getState().setSnapshot(snap("disconnected")));
  act(() => useGhost.getState().setAutotune({ running: false, error: { code: "NOT_CONNECTED" } } as any));
  await new Promise((r) => setTimeout(r, 20));
  expect(svc.MarkNetworkChecked).not.toHaveBeenCalled();
  act(() => useGhost.getState().setSnapshot(snap("protected", true, ["youtube.com"])));
  await waitFor(() => expect(svc.StartAutotune).toHaveBeenCalledTimes(2));
});

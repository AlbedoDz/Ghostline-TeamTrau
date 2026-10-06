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

const snap = (status: string) =>
  ({ status, step: 0, warnings: [], servers: ["Cloudflare"], since: new Date().toISOString(), latencyMs: 20, queries: 0,
    blockedSites: [], reasons: [], dpi: { enabled: false, running: false } }) as any;

const res = (site: string, stage: string) => ({ site, stage, latency: 0 });
const allOK = [res("youtube.com", "ok"), res("discord.com", "ok"), res("x.com", "ok")];
const blocked = [res("youtube.com", "tls"), res("discord.com", "ok"), res("x.com", "ok")];

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  useGhost.getState().reset();
});

test("first connect: blocked test sites start auto-tune, shown as a step with its progress", async () => {
  svc.ProbeNow.mockResolvedValue(blocked);
  useGhost.getState().setSettings(settings(false));
  useGhost.getState().setSnapshot(snap("protected"));
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.getByText(/kiểm tra trang mẫu/)).toBeInTheDocument();
  await waitFor(() => expect(svc.StartAutotune).toHaveBeenCalled());
  expect(svc.ProbeNow, "blocked twice before tuning").toHaveBeenCalledTimes(2);

  act(() => useGhost.getState().setAutotune({ running: true, engine: "zapret2", preset: "z-split", index: 12, total: 900 } as any));
  expect(screen.getByText(/tự dò vượt DPI với trang mẫu 12\/900/)).toBeInTheDocument();
  expect(screen.getByText("đặt DNS").closest("[data-step]")).toHaveAttribute("data-step", "done");
  expect(screen.queryByText(/đang dò:/), "no separate banner").toBeNull();
  expect(svc.MarkNetworkChecked).not.toHaveBeenCalled();

  act(() => useGhost.getState().setAutotune({ running: false, engine: "zapret2", preset: "z-split", index: 0, total: 0 } as any));
  await waitFor(() => expect(svc.MarkNetworkChecked).toHaveBeenCalled());
  expect(useGhost.getState().settings?.simple?.checked).toBe(true);
  expect(screen.queryByText(/tự dò vượt DPI với trang mẫu/)).toBeNull();
});

test("first connect: nothing blocked, nothing tuned", async () => {
  svc.ProbeNow.mockResolvedValue(allOK);
  useGhost.getState().setSettings(settings(false));
  useGhost.getState().setSnapshot(snap("protected"));
  render(<SimpleView onOpenLogs={() => {}} />);
  await waitFor(() => expect(svc.MarkNetworkChecked).toHaveBeenCalled());
  expect(svc.StartAutotune).not.toHaveBeenCalled();
  expect(svc.ProbeNow).toHaveBeenCalledTimes(1);
});

test("a site blocked only once is a fluke", async () => {
  svc.ProbeNow.mockResolvedValueOnce(blocked).mockResolvedValueOnce(allOK);
  useGhost.getState().setSettings(settings(false));
  useGhost.getState().setSnapshot(snap("protected"));
  render(<SimpleView onOpenLogs={() => {}} />);
  await waitFor(() => expect(svc.MarkNetworkChecked).toHaveBeenCalled());
  expect(svc.StartAutotune).not.toHaveBeenCalled();
});

test("only on the first run, and only once connected", async () => {
  useGhost.getState().setSettings(settings(true));
  useGhost.getState().setSnapshot(snap("protected"));
  const { unmount } = render(<SimpleView onOpenLogs={() => {}} />);
  unmount();
  useGhost.getState().setSettings(settings(false));
  useGhost.getState().setSnapshot(snap("disconnected"));
  render(<SimpleView onOpenLogs={() => {}} />);
  await new Promise((r) => setTimeout(r, 20));
  expect(svc.ProbeNow).not.toHaveBeenCalled();
});

test("disconnecting mid-way leaves it for the next connect", async () => {
  svc.ProbeNow.mockResolvedValue(blocked);
  useGhost.getState().setSettings(settings(false));
  useGhost.getState().setSnapshot(snap("protected"));
  render(<SimpleView onOpenLogs={() => {}} />);
  await waitFor(() => expect(svc.StartAutotune).toHaveBeenCalledTimes(1));
  act(() => useGhost.getState().setAutotune({ running: true, index: 3, total: 900 } as any));
  act(() => useGhost.getState().setSnapshot(snap("disconnected")));
  act(() => useGhost.getState().setAutotune({ running: false, error: { code: "NOT_CONNECTED" } } as any));
  await new Promise((r) => setTimeout(r, 20));
  expect(svc.MarkNetworkChecked).not.toHaveBeenCalled();
  act(() => useGhost.getState().setSnapshot(snap("protected")));
  await waitFor(() => expect(svc.StartAutotune).toHaveBeenCalledTimes(2));
});

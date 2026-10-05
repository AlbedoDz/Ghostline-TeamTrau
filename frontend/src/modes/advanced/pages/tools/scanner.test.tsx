import { act } from "react";
import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { Scanner } from "./Scanner";
import { useGhost } from "../../../../app/store";
import { initI18n } from "../../../../i18n";

const row = (id: string, extra: Record<string, unknown> = {}, result: Record<string, unknown> = {}) => ({
  server: { id, name: id.toUpperCase(), protocol: "doh", address: `https://${id}.example/dns-query`, source: "custom", provider: "", tags: [] },
  pinned: false, pasted: false,
  result: { serverId: id, reach: { ok: true }, minMs: 10, medianMs: 20, p90Ms: 30, jitterMs: 2, loss: 0, dnssec: "yes", adFilter: "no", poisoned: [], ...result },
  ...extra,
});

const svc = vi.hoisted(() => ({
  StartAdvancedScan: vi.fn(() => Promise.resolve({ total: 3, bad: [] as string[] })),
  CancelAdvancedScan: vi.fn(() => Promise.resolve()),
  AdvancedResults: vi.fn(() => Promise.resolve([] as any[])),
  AddScannedServers: vi.fn(() => Promise.resolve(1)),
  ExportAdvancedCSV: vi.fn(() => Promise.resolve()),
  SetPinnedMany: vi.fn(() => Promise.resolve()),
  UseOnlyServer: vi.fn(() => Promise.resolve()),
  SaveSettings: vi.fn(() => Promise.resolve()),
}));
vi.mock("../../../../app/api", () => ({ Service: svc }));

const settings = {
  language: "vi", pinned: [], probeSites: ["youtube.com"],
  tools: { scanner: { rounds: 5, workers: 8, timeoutMs: 3000 }, cfscan: { host: "speed.cloudflare.com", maxIps: 2000, want: 50, concurrency: 64, timeoutMs: 2000, speedTest: true, speedBytes: 1048576 } },
};

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  useGhost.getState().reset();
  useGhost.getState().setSettings(structuredClone(settings) as any);
});

test("start, progress from events, cancel", async () => {
  render(<Scanner />);
  fireEvent.click(screen.getByRole("button", { name: "quét" }));
  await waitFor(() => expect(svc.StartAdvancedScan).toHaveBeenCalled());
  expect((svc.StartAdvancedScan.mock.calls[0] as any[])[0].filter).toBeTruthy();
  act(() => useGhost.getState().setAdvScan({ done: 1, total: 3, running: true }));
  expect(await screen.findByText("1/3")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "huỷ" }));
  expect(svc.CancelAdvancedScan).toHaveBeenCalled();
});

test("results load when the scan ends; filters, bulk pin, use only, add pasted, export", async () => {
  svc.AdvancedResults.mockResolvedValue([
    row("aa"),
    row("bb", { pasted: true }, { dnssec: "no" }),
  ]);
  render(<Scanner />);
  act(() => useGhost.getState().setAdvScan({ done: 2, total: 2, running: false }));
  expect(await screen.findByText("AA")).toBeInTheDocument();
  expect(screen.getByText("BB")).toBeInTheDocument();

  fireEvent.click(screen.getByRole("checkbox", { name: "chỉ server có DNSSEC" }));
  expect(screen.queryByText("BB")).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("checkbox", { name: "chỉ server có DNSSEC" }));

  fireEvent.click(screen.getByRole("checkbox", { name: "chọn AA" }));
  fireEvent.click(screen.getByRole("button", { name: "ghim mục đã chọn" }));
  await waitFor(() => expect(svc.SetPinnedMany).toHaveBeenCalledWith(["aa"], true));

  const aa = screen.getByText("AA").closest("tr")!;
  fireEvent.click(within(aa).getByRole("button", { name: "chỉ dùng" }));
  await waitFor(() => expect(svc.UseOnlyServer).toHaveBeenCalledWith("aa"));

  const bb = screen.getByText("BB").closest("tr")!;
  fireEvent.click(within(bb).getByRole("button", { name: "thêm" }));
  await waitFor(() => expect(svc.AddScannedServers).toHaveBeenCalledWith(["bb"]));

  fireEvent.click(screen.getByRole("button", { name: "xuất CSV" }));
  expect(svc.ExportAdvancedCSV).toHaveBeenCalled();
});

test("SCAN_TOO_MANY is shown", async () => {
  svc.StartAdvancedScan.mockRejectedValueOnce(new Error("SCAN_TOO_MANY"));
  render(<Scanner />);
  fireEvent.click(screen.getByRole("radio", { name: "dán danh sách" }));
  fireEvent.change(screen.getByRole("textbox", { name: "server cần quét" }), { target: { value: "https://a.example/dns-query" } });
  fireEvent.click(screen.getByRole("button", { name: "quét" }));
  expect(await screen.findByText(/tối đa 500/)).toBeInTheDocument();
  expect((svc.StartAdvancedScan.mock.calls[0] as any[])[0]).toEqual({ pasted: "https://a.example/dns-query", poisonDomains: ["youtube.com"] });
});

test("options save tools.scanner and show range errors", async () => {
  svc.SaveSettings.mockRejectedValueOnce(new Error("tools: scanner rounds must be 3..20"));
  render(<Scanner />);
  fireEvent.click(screen.getByText("tuỳ chọn"));
  const rounds = screen.getByRole("spinbutton", { name: "số lượt đo" });
  fireEvent.change(rounds, { target: { value: "30" } });
  fireEvent.blur(rounds);
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalled());
  expect((svc.SaveSettings.mock.calls[0] as any[])[0].tools.scanner.rounds).toBe(30);
  expect(await screen.findByText(/3\.\.20/)).toBeInTheDocument();
});

import { act } from "react";
import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { CfScan } from "./CfScan";
import { useGhost } from "../../../../app/store";
import { initI18n } from "../../../../i18n";

const ip = (n: number, extra: Record<string, unknown> = {}) => ({ ip: `104.16.0.${n}`, ok: true, latencyMs: 40 + n, colo: "HKG", mbps: 0, checkedAt: "2026-10-06T08:00:00Z", ...extra });

const svc = vi.hoisted(() => ({
  GetCFView: vi.fn(),
  StartCFScan: vi.fn(() => Promise.resolve()),
  CancelCFScan: vi.fn(() => Promise.resolve()),
  RecheckCF: vi.fn(() => Promise.resolve([] as any[])),
  CFSuggestDomains: vi.fn(() => Promise.resolve(["youtube.com", "*.example.com"])),
  CreateCFRules: vi.fn(() => Promise.resolve([] as any[])),
  SaveSettings: vi.fn(() => Promise.resolve()),
}));
vi.mock("../../../../app/api", () => ({ Service: svc }));

const settings = {
  language: "vi",
  tools: { scanner: { rounds: 5, workers: 8, timeoutMs: 3000 }, cfscan: { host: "speed.cloudflare.com", maxIps: 2000, want: 50, concurrency: 64, timeoutMs: 2000, speedTest: true, speedBytes: 1048576 } },
};
const writeText = vi.fn(() => Promise.resolve());

beforeAll(() => {
  initI18n("vi");
  Object.assign(navigator, { clipboard: { writeText } });
});
beforeEach(() => {
  vi.clearAllMocks();
  useGhost.getState().reset();
  useGhost.getState().setSettings(structuredClone(settings) as any);
  svc.GetCFView.mockResolvedValue({ scannedAt: "2026-10-06T08:00:00Z", host: "speed.cloudflare.com", running: false, results: [ip(1, { mbps: 55.5 }), ip(2), ip(3), ip(4), ip(5)] });
});

test("the cached scan is shown with its time", async () => {
  render(<CfScan />);
  expect(await screen.findByText("104.16.0.1")).toBeInTheDocument();
  expect(screen.getByText(/đã quét lúc/)).toBeInTheDocument();
  expect(screen.getByText("55.5")).toBeInTheDocument();
});

test("progress from events, probe then speed, and the no-speed note", async () => {
  render(<CfScan />);
  fireEvent.click(screen.getByRole("button", { name: "quét" }));
  await waitFor(() => expect(svc.StartCFScan).toHaveBeenCalled());
  act(() => useGhost.getState().setCfScan({ phase: "probe", tried: 40, ok: 7, total: 2000, running: true }));
  expect(await screen.findByText(/40\/2000/)).toBeInTheDocument();
  act(() => useGhost.getState().setCfScan({ phase: "speed", tried: 60, ok: 50, total: 2000, running: true }));
  expect(await screen.findByText(/đang đo tốc độ/)).toBeInTheDocument();
  act(() => useGhost.getState().setCfScan({ phase: "speed", tried: 60, ok: 50, total: 2000, running: false, note: "no_speed_endpoint" }));
  expect(await screen.findByText(/không có \/__down/)).toBeInTheDocument();
  await waitFor(() => expect(svc.GetCFView).toHaveBeenCalledTimes(2));
});

test("a failed scan shows its error", async () => {
  render(<CfScan />);
  act(() => useGhost.getState().setCfScan({ phase: "probe", tried: 200, ok: 0, total: 2000, running: false, error: "CFSCAN_NO_NETWORK" }));
  expect(await screen.findByText(/Không kết nối được tới IP Cloudflare nào/)).toBeInTheDocument();
});

test("copy writes the selected IPs one per line", async () => {
  render(<CfScan />);
  fireEvent.click(await screen.findByRole("checkbox", { name: "chọn 104.16.0.1" }));
  fireEvent.click(screen.getByRole("checkbox", { name: "chọn 104.16.0.2" }));
  fireEvent.click(screen.getByRole("button", { name: "sao chép" }));
  expect(writeText).toHaveBeenCalledWith("104.16.0.1\n104.16.0.2");
});

test("create rule dialog: suggestions, IP cap, line errors, success", async () => {
  svc.CreateCFRules.mockResolvedValueOnce([{ line: 1, msg: "\"~x\" is not a domain pattern" }]);
  render(<CfScan />);
  for (const n of [1, 2, 3, 4, 5]) fireEvent.click(await screen.findByRole("checkbox", { name: `chọn 104.16.0.${n}` }));
  fireEvent.click(screen.getByRole("button", { name: "tạo rule" }));
  const dialog = await screen.findByRole("dialog");
  expect(dialog).toHaveTextContent(/chỉ có tác dụng với domain thật sự nằm sau Cloudflare/);
  const submit = screen.getByRole("button", { name: "tạo" });
  expect(submit).toBeDisabled(); // 5 IPs > 4
  fireEvent.click(within(dialog).getByRole("checkbox", { name: "chọn 104.16.0.5" }));
  fireEvent.click(await within(dialog).findByRole("button", { name: "youtube.com" }));
  const box = screen.getByRole("textbox", { name: "domain" }) as HTMLTextAreaElement;
  expect(box.value).toBe("youtube.com");
  fireEvent.change(box, { target: { value: "~x" } });
  fireEvent.click(screen.getByRole("button", { name: "tạo" }));
  expect(await screen.findByText(/is not a domain pattern/)).toBeInTheDocument();
  expect(svc.CreateCFRules).toHaveBeenCalledWith(["~x"], ["104.16.0.1", "104.16.0.2", "104.16.0.3", "104.16.0.4"]);
  fireEvent.change(box, { target: { value: "youtube.com\n*.example.com" } });
  fireEvent.click(screen.getByRole("button", { name: "tạo" }));
  await waitFor(() => expect(screen.queryByRole("dialog")).not.toBeInTheDocument());
  expect(screen.getByText(/Đã tạo rule/)).toBeInTheDocument();
});

test("options save tools.cfscan; restore default host", async () => {
  render(<CfScan />);
  fireEvent.click(screen.getByText("tuỳ chọn"));
  const host = screen.getByRole("textbox", { name: "host kiểm tra" });
  fireEvent.change(host, { target: { value: "1.1.1.1" } });
  fireEvent.blur(host);
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalled());
  expect((svc.SaveSettings.mock.calls[0] as any[])[0].tools.cfscan.host).toBe("1.1.1.1");
  fireEvent.click(screen.getByRole("button", { name: "khôi phục mặc định" }));
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalledTimes(2));
  expect((svc.SaveSettings.mock.calls[1] as any[])[0].tools.cfscan.host).toBe("speed.cloudflare.com");
  fireEvent.click(screen.getByRole("checkbox", { name: "đo tốc độ tải" }));
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalledTimes(3));
  expect((svc.SaveSettings.mock.calls[2] as any[])[0].tools.cfscan.speedTest).toBe(false);
});

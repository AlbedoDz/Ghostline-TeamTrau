import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { DnsServer } from "./DnsServer";
import { Overview } from "./Overview";
import { SimpleView } from "../../simple/SimpleView";
import { useGhost } from "../../../app/store";
import { initI18n } from "../../../i18n";

const device = {
  dnsAddrs: ["192.168.1.5"], dohUrls: ["https://192.168.1.5/dns-query"], fingerprint: "AB:CD:EF",
  ssid: "Nhà", public: false, setupUrl: "", setupRemainingSec: 0, wifiSuggestions: ["Nhà", "Nhà 5G", "Hàng xóm"],
};

const svc = vi.hoisted(() => ({
  SaveSettings: vi.fn(() => Promise.resolve()),
  SetDNSServer: vi.fn(() => Promise.resolve()),
  SetIOSSSID: vi.fn(() => Promise.resolve()),
  GetDeviceInfo: vi.fn(),
  OpenSetupPage: vi.fn(() => Promise.resolve("http://192.168.1.5:8053/")),
  CloseSetupPage: vi.fn(() => Promise.resolve()),
  SaveDeviceFiles: vi.fn(() => Promise.resolve()),
  ResetLANCA: vi.fn(() => Promise.resolve()),
  RemoveLANCA: vi.fn(() => Promise.resolve()),
  GetQR: vi.fn(() => Promise.resolve([[true, false], [false, true]])),
  ListCerts: vi.fn(() => Promise.resolve([{ thumbprint: "aa", subject: "Ghostline LAN CA — PC ABCD", notAfter: "2031-10-05T00:00:00Z" }])),
  GetSettings: vi.fn(),
  GetProxyStats: vi.fn(() => Promise.resolve(null)),
  Connect: vi.fn(() => Promise.resolve()),
  Disconnect: vi.fn(() => Promise.resolve()),
  CancelConnect: vi.fn(() => Promise.resolve()),
  SetMode: vi.fn(() => Promise.resolve()),
}));
vi.mock("../../../app/api", () => ({ Service: svc }));
vi.mock("@wailsio/runtime", () => ({ Browser: { OpenURL: vi.fn() } }));

const settings = {
  version: 4, language: "vi", mode: "advanced", probeSites: [], bootstrap: ["1.1.1.1:53"], pinned: [],
  dpi: { enabled: false, preset: "light", customArgs: "", scope: "all" },
  fragmentDns: { enabled: false, chunks: 5, delayMs: 5 },
  proxy: { enabled: true, port: 8080, systemProxy: false, shareLan: false, fragment: { mode: "auto", method: "both", chunks: 5, delayMs: 5, autoTimeoutMs: 3000, cacheDays: 7 }, upstreams: [] },
  dnsBlockMode: "zero",
  dnsServer: { enabled: true, shareLan: true, dohPort: 443, iosSsid: "" },
  fakeSni: { enabled: false, ackVersion: 0 },
};

const running = {
  status: "protected", warnings: [], servers: ["Cloudflare"], blockedSites: [], reasons: [],
  dpi: { enabled: false, running: false, preset: "light" },
  proxy: { running: false, addr: "", systemProxy: false, shareLan: false },
  dnsServer: { running: true, addrs: ["127.0.0.1:443", "192.168.1.5:443"], skipped: { "10.0.0.7:53": "in use" } },
  fakeSni: { active: false, domains: 0, needsProxy: false, notAfter: "" },
};

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  svc.GetDeviceInfo.mockResolvedValue({ ...device });
  useGhost.getState().reset();
  useGhost.getState().setSettings(structuredClone(settings) as any);
  useGhost.getState().setSnapshot(structuredClone(running) as any);
});

test("toggles and the DoH port save through SetDNSServer", async () => {
  render(<DnsServer />);
  fireEvent.click(screen.getByRole("switch", { name: "chia sẻ cho LAN" }));
  await waitFor(() => expect(svc.SetDNSServer).toHaveBeenCalledWith(true, false, 443));
  const port = screen.getByRole("spinbutton", { name: "cổng DoH" });
  fireEvent.change(port, { target: { value: "8053" } });
  fireEvent.blur(port);
  expect(await screen.findByText(/cổng DoH phải khác 53, 8053/)).toBeInTheDocument();
  expect(svc.SetDNSServer).toHaveBeenCalledTimes(1);
});

test("shows device addresses, the fingerprint and skipped addresses", async () => {
  render(<DnsServer />);
  expect(await screen.findByText("https://192.168.1.5/dns-query")).toBeInTheDocument();
  expect(screen.getByText("AB:CD:EF")).toBeInTheDocument();
  expect(screen.getByText(/10\.0\.0\.7:53/)).toBeInTheDocument();
  expect(screen.getByText(/in use/)).toBeInTheDocument();
});

test("the phone setup page shows a QR code and a countdown, then hides", async () => {
  render(<DnsServer />);
  fireEvent.click(await screen.findByRole("button", { name: "mở trang cài đặt cho điện thoại" }));
  await waitFor(() => expect(svc.OpenSetupPage).toHaveBeenCalled());
  act(() => useGhost.getState().setSetup({ url: "http://192.168.1.5:8053/", remainingSec: 125 }));
  expect(await screen.findByRole("img", { name: /192\.168\.1\.5:8053/ })).toBeInTheDocument();
  expect(screen.getByText(/2:05/)).toBeInTheDocument();
  act(() => useGhost.getState().setSetup({ url: "", remainingSec: 0 }));
  await waitFor(() => expect(screen.queryByRole("img", { name: /8053/ })).not.toBeInTheDocument());
});

test("an empty home Wi-Fi name is warned about while sharing", async () => {
  svc.GetDeviceInfo.mockResolvedValue({ ...device, ssid: "", wifiSuggestions: [] });
  render(<DnsServer />);
  expect(await screen.findByText(/chưa có tên Wi-Fi nhà/i)).toBeInTheDocument();
  fireEvent.change(screen.getByRole("combobox", { name: "tên Wi-Fi nhà" }), { target: { value: "Home" } });
  fireEvent.blur(screen.getByRole("combobox", { name: "tên Wi-Fi nhà" }));
  await waitFor(() => expect(svc.SetIOSSSID).toHaveBeenCalledWith("Home"));
});

test("a Public network shows the hint", async () => {
  svc.GetDeviceInfo.mockResolvedValue({ ...device, public: true });
  render(<DnsServer />);
  expect(await screen.findByText(/Mạng đang là Public/)).toBeInTheDocument();
});

test("removing the LAN CA asks first", async () => {
  const confirm = vi.spyOn(window, "confirm").mockReturnValueOnce(false).mockReturnValueOnce(true);
  render(<DnsServer />);
  const btn = await screen.findByRole("button", { name: "gỡ CA LAN" });
  fireEvent.click(btn);
  expect(svc.RemoveLANCA).not.toHaveBeenCalled();
  fireEvent.click(btn);
  await waitFor(() => expect(svc.RemoveLANCA).toHaveBeenCalled());
  confirm.mockRestore();
});

test("the server error is shown with its code text", async () => {
  useGhost.getState().setSnapshot({ ...structuredClone(running), status: "degraded", reasons: ["dnsserver"],
    dnsServer: { running: false, addrs: [], error: { code: "DNSSERVER_PORT_IN_USE", params: { port: 443, addrs: ["127.0.0.1:443"] } } } } as any);
  render(<DnsServer />);
  expect(await screen.findByText(/Cổng 443 đang bị chiếm/)).toBeInTheDocument();
});

test("overview and simple mode show the DNS server", async () => {
  render(<Overview />);
  expect(await screen.findByText(/DNS server/i)).toBeInTheDocument();
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.getByText(/DNS cho LAN/)).toBeInTheDocument();
  expect(screen.getAllByText(/192\.168\.1\.5/).length).toBeGreaterThan(0);
});

test("Wi-Fi names to pick from are offered", async () => {
  render(<DnsServer />);
  const input = await screen.findByRole("combobox", { name: "tên Wi-Fi nhà" });
  const list = document.getElementById(input.getAttribute("list")!)!;
  expect(Array.from(list.querySelectorAll("option")).map((o) => o.getAttribute("value"))).toEqual(["Nhà", "Nhà 5G", "Hàng xóm"]);
});

test("no warning once a home Wi-Fi name is saved", async () => {
  useGhost.getState().setSettings({ ...structuredClone(settings), dnsServer: { ...settings.dnsServer, iosSsid: "Nhà" } } as any);
  render(<DnsServer />);
  await screen.findByText("https://192.168.1.5/dns-query");
  expect(screen.queryByText(/chưa có tên Wi-Fi nhà/i)).not.toBeInTheDocument();
});

test("while disconnected the setup button says why it is off", async () => {
  useGhost.getState().setSnapshot({ ...structuredClone(running), status: "disconnected", dnsServer: { running: false, addrs: [] } } as any);
  render(<DnsServer />);
  expect(await screen.findByRole("button", { name: "mở trang cài đặt cho điện thoại" })).toBeDisabled();
  expect(screen.getByText("Cần kết nối và bật chia sẻ LAN để mở trang cài đặt.")).toBeInTheDocument();
});

test("no hint while the setup page can be opened", async () => {
  render(<DnsServer />);
  expect(await screen.findByRole("button", { name: "mở trang cài đặt cho điện thoại" })).toBeEnabled();
  expect(screen.queryByText(/Cần kết nối và bật chia sẻ LAN/)).not.toBeInTheDocument();
});

import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { SimpleView } from "./SimpleView";
import { useGhost } from "../../app/store";
import { initI18n } from "../../i18n";
import { levelOf } from "../../app/protection";

const svc = vi.hoisted(() => ({
  Connect: vi.fn(() => Promise.resolve()),
  SaveSettings: vi.fn(() => Promise.resolve()),
  SetFakeSNI: vi.fn(() => Promise.resolve()),
  SetDPIEnabled: vi.fn(() => Promise.resolve()),
  MarkNetworkChecked: vi.fn(() => Promise.resolve()),
  GetSettings: vi.fn(() => Promise.resolve(null)),
  DPIStrategies: vi.fn(() => Promise.resolve([])),
}));
vi.mock("../../app/api", () => ({ Service: svc }));
vi.mock("@wailsio/runtime", () => ({ Browser: { OpenURL: vi.fn() } }));

const base = {
  probeSites: [], fragmentDns: { enabled: false, chunks: 5, delayMs: 5 },
  dpi: { enabled: false, engine: "zapret2", preset: "light", scope: "all" },
  proxy: { enabled: false, systemProxy: false, port: 8080, upstreams: [] },
  fakeSni: { enabled: false, ackVersion: 1 },
  simple: {},
};

function settings(over: { dpi?: boolean; proxy?: boolean; sys?: boolean; fsni?: boolean; custom?: object } = {}) {
  const s = structuredClone(base) as any;
  s.dpi.enabled = !!over.dpi;
  s.proxy.enabled = !!over.proxy;
  s.proxy.systemProxy = !!over.sys;
  s.fakeSni.enabled = !!over.fsni;
  if (over.custom) s.simple.custom = over.custom;
  return s;
}

const lastSaved = () => (svc.SaveSettings.mock.calls[svc.SaveSettings.mock.calls.length - 1] as any[])[0];

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  useGhost.getState().reset();
  useGhost.getState().setSnapshot({ status: "disconnected", warnings: [], servers: [], blockedSites: [], reasons: [], dpi: { enabled: false } } as any);
});

test("levelOf reads the level from the settings", () => {
  expect(levelOf(settings())).toBe("dns");
  expect(levelOf(settings({ dpi: true }))).toBe("dpi");
  expect(levelOf(settings({ dpi: true, proxy: true, sys: true }))).toBe("max");
  expect(levelOf(settings({ dpi: true, proxy: true, sys: true, fsni: true }))).toBe("max");
  expect(levelOf(settings({ dpi: true, proxy: true }))).toBe("custom");
  expect(levelOf(settings({ proxy: true, sys: true }))).toBe("custom");
  expect(levelOf({ dpi: { enabled: false } } as any), "older settings without proxy").toBe("dns");
});

test("the current level is shown and a fresh install is DNS only", () => {
  useGhost.getState().setSettings(settings());
  render(<SimpleView onOpenLogs={() => {}} />);
  const group = screen.getByRole("radiogroup", { name: "mức bảo vệ" });
  expect(group).toBeInTheDocument();
  expect(screen.getByRole("radio", { name: "Chỉ DNS" })).toHaveAttribute("aria-checked", "true");
  expect(screen.getByRole("radio", { name: "DNS + vượt DPI" }).getAttribute("title")).toMatch(/khuyên dùng/i);
});

test("choosing a level saves only its switches", async () => {
  useGhost.getState().setSettings(settings());
  render(<SimpleView onOpenLogs={() => {}} />);
  fireEvent.click(screen.getByRole("radio", { name: "DNS + vượt DPI" }));
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalled());
  expect(lastSaved().dpi).toEqual({ ...base.dpi, enabled: true });
  expect(lastSaved().proxy.enabled).toBe(false);
  fireEvent.click(screen.getByRole("radio", { name: "Tối đa" }));
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalledTimes(2));
  expect(lastSaved().dpi.enabled).toBe(true);
  expect(lastSaved().proxy).toMatchObject({ enabled: true, systemProxy: true, port: 8080 });
  expect(lastSaved().simple.custom, "a level never overwrites the remembered custom").toBeUndefined();
  expect(screen.getByRole("radio", { name: "Tối đa" })).toHaveAttribute("aria-checked", "true");
});

test("leaving custom remembers it, and custom brings it back", async () => {
  useGhost.getState().setSettings(settings({ dpi: true, proxy: true, sys: false }));
  render(<SimpleView onOpenLogs={() => {}} />);
  expect(screen.getByRole("radio", { name: "Tuỳ chỉnh" })).toHaveAttribute("aria-checked", "true");
  expect(screen.getByText(/vượt DPI \+ proxy \(không dùng cho máy này\)/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("radio", { name: "Chỉ DNS" }));
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalled());
  expect(lastSaved().simple.custom).toEqual({ dpi: true, proxy: true, systemProxy: false, fakeSni: false });

  fireEvent.click(screen.getByRole("radio", { name: "Tuỳ chỉnh" }));
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalledTimes(2));
  expect(lastSaved().dpi.enabled).toBe(true);
  expect(lastSaved().proxy).toMatchObject({ enabled: true, systemProxy: false });
});

test("custom with nothing remembered opens the full interface", () => {
  useGhost.getState().setSettings(settings());
  const onOpenFull = vi.fn();
  render(<SimpleView onOpenLogs={() => {}} onOpenFull={onOpenFull} />);
  fireEvent.click(screen.getByRole("radio", { name: "Tuỳ chỉnh" }));
  expect(onOpenFull).toHaveBeenCalled();
  expect(svc.SaveSettings).not.toHaveBeenCalled();
});

test("turning the proxy off while Fake SNI runs asks first, and custom turns Fake SNI back on", async () => {
  useGhost.getState().setSettings(settings({ dpi: true, proxy: true, sys: true, fsni: true }));
  const confirm = vi.spyOn(window, "confirm").mockReturnValueOnce(false);
  render(<SimpleView onOpenLogs={() => {}} />);
  fireEvent.click(screen.getByRole("radio", { name: "Chỉ DNS" }));
  expect(confirm).toHaveBeenCalled();
  expect(svc.SaveSettings).not.toHaveBeenCalled();

  confirm.mockReturnValueOnce(true);
  fireEvent.click(screen.getByRole("radio", { name: "Chỉ DNS" }));
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalled());
  expect(svc.SetFakeSNI).toHaveBeenCalledWith(false);
  expect(svc.SetFakeSNI.mock.invocationCallOrder[0]).toBeLessThan(svc.SaveSettings.mock.invocationCallOrder[0]);
  expect(lastSaved().simple.custom, "Max with Fake SNI is remembered as custom").toEqual({ dpi: true, proxy: true, systemProxy: true, fakeSni: true });

  fireEvent.click(screen.getByRole("radio", { name: "Tuỳ chỉnh" }));
  await waitFor(() => expect(svc.SetFakeSNI).toHaveBeenLastCalledWith(true));
  expect(svc.SaveSettings.mock.invocationCallOrder[1]).toBeLessThan(svc.SetFakeSNI.mock.invocationCallOrder[1]);
  confirm.mockRestore();
});

test("an error is shown and the level stays", async () => {
  useGhost.getState().setSettings(settings());
  svc.SetDPIEnabled.mockRejectedValueOnce(new Error("DPI_START_FAILED"));
  render(<SimpleView onOpenLogs={() => {}} />);
  fireEvent.click(screen.getByRole("radio", { name: "DNS + vượt DPI" }));
  expect(await screen.findByRole("alert")).toBeInTheDocument();
  expect(screen.getByRole("radio", { name: "Chỉ DNS" })).toHaveAttribute("aria-checked", "true");
});

test("DPI is switched through SetDPIEnabled, which really starts the engine", async () => {
  useGhost.getState().setSettings(settings());
  let finish: (v?: any) => void = () => {};
  svc.SetDPIEnabled.mockImplementationOnce(() => new Promise((r) => (finish = r)));
  render(<SimpleView onOpenLogs={() => {}} />);
  fireEvent.click(screen.getByRole("radio", { name: "DNS + vượt DPI" }));
  // While the engine starts, the chosen level shows at once and says so.
  expect(await screen.findByRole("radio", { name: "DNS + vượt DPI" })).toHaveAttribute("aria-checked", "true");
  expect(screen.getByTestId("level-description")).toHaveTextContent(/đang áp dụng/i);
  // A snapshot from before the switch must not flip it back.
  act(() => useGhost.getState().setSnapshot({ status: "protected", warnings: [], servers: [], blockedSites: [], reasons: [], dpi: { enabled: false, running: false } } as any));
  expect(screen.getByRole("radio", { name: "DNS + vượt DPI" })).toHaveAttribute("aria-checked", "true");
  finish();
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalled());
  expect(svc.SetDPIEnabled).toHaveBeenCalledWith(true);
  expect(svc.SetDPIEnabled.mock.invocationCallOrder[0]).toBeLessThan(svc.SaveSettings.mock.invocationCallOrder[0]);
  await waitFor(() => expect(screen.getByTestId("level-description")).toHaveTextContent("Thêm vượt DPI"));
  expect(screen.getByRole("radio", { name: "DNS + vượt DPI" })).toHaveAttribute("aria-checked", "true");

  fireEvent.click(screen.getByRole("radio", { name: "Tối đa" }));
  await waitFor(() => expect(svc.SaveSettings).toHaveBeenCalledTimes(2));
  expect(svc.SetDPIEnabled, "already on: not switched again").toHaveBeenCalledTimes(1);
});

test("a line under the levels describes the current one", async () => {
  useGhost.getState().setSettings(settings());
  render(<SimpleView onOpenLogs={() => {}} />);
  const desc = screen.getByTestId("level-description");
  expect(desc).toHaveTextContent("Chỉ mã hoá DNS");
  fireEvent.click(screen.getByRole("radio", { name: "DNS + vượt DPI" }));
  await waitFor(() => expect(desc).toHaveTextContent("Thêm vượt DPI cho mọi ứng dụng (khuyên dùng)"));
  expect(screen.getByRole("radiogroup", { name: "mức bảo vệ" })).toHaveAttribute("aria-describedby", desc.id);
});


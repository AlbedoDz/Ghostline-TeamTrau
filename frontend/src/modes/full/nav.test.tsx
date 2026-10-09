import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { act, fireEvent, render, screen, within } from "@testing-library/react";
import { FullView } from "./FullView";
import { useGhost } from "../../app/store";
import { initI18n } from "../../i18n";

vi.mock("../../app/api", () => ({ Service: {} }));
const { stub } = vi.hoisted(() => ({ stub: (name: string) => () => name + " PAGE" }));
vi.mock("./pages/Overview", () => ({ Overview: stub("OVERVIEW") }));
vi.mock("./pages/Servers", () => ({ Servers: stub("SERVERS") }));
vi.mock("./pages/Dpi", () => ({ Dpi: stub("DPI") }));
vi.mock("./pages/Proxy", () => ({ Proxy: stub("PROXY") }));
vi.mock("./pages/Rules", () => ({ Rules: stub("RULES") }));
vi.mock("./pages/DnsServer", () => ({ DnsServer: stub("DNSSERVER") }));
vi.mock("./pages/FakeSni", () => ({ FakeSni: stub("FAKESNI") }));
vi.mock("./pages/Logs", () => ({ Logs: stub("LOGS") }));
vi.mock("./pages/Settings", () => ({ Settings: stub("SETTINGS") }));
vi.mock("./pages/tools/Lookup", () => ({ Lookup: stub("LOOKUP") }));
vi.mock("./pages/tools/Scanner", () => ({ Scanner: stub("SCANNER") }));
vi.mock("./pages/tools/CfScan", () => ({ CfScan: stub("CFSCAN") }));
vi.mock("./pages/tools/Stamp", () => ({ Stamp: stub("STAMP") }));
vi.mock("./pages/tools/Gaming", () => ({ Gaming: stub("GAMING") }));
vi.mock("../../components/Warnings", () => ({ Warnings: () => null }));
vi.mock("../../components/ConnectError", () => ({ ConnectError: () => null }));

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  useGhost.getState().reset();
  useGhost.getState().setSnapshot({ status: "disconnected", warnings: [], servers: [], blockedSites: [], reasons: [], dpi: { enabled: false } } as any);
});

test("the sidebar has 8 pages in three groups; fake sni and logs are tabs", () => {
  render(<FullView />);
  const nav = screen.getByRole("navigation");
  const pages = within(nav).getAllByRole("button").map((b) => b.textContent).filter((t) => t && !/kết nối/i.test(t));
  expect(pages).toEqual(["tổng quan", "máy chủ", "vượt dpi", "proxy", "rules", "dns server", "công cụ", "cài đặt"]);
  for (const h of ["cơ bản", "nâng cao", "chẩn đoán"]) expect(within(nav).getByText(h)).toBeInTheDocument();
  expect(within(nav).getByRole("separator"), "settings is set apart").toBeInTheDocument();
});

test("proxy has a fake sni tab", () => {
  render(<FullView />);
  fireEvent.click(screen.getByRole("button", { name: "proxy" }));
  expect(screen.getByText("PROXY PAGE")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("tab", { name: "fake sni" }));
  expect(screen.getByText("FAKESNI PAGE")).toBeInTheDocument();
});

test("tools opens on the logs tab, which comes first", () => {
  render(<FullView />);
  fireEvent.click(screen.getByRole("button", { name: "công cụ" }));
  const tabs = screen.getAllByRole("tab").map((t) => t.textContent);
  expect(tabs).toEqual(["nhật ký", "chơi game", "lookup", "scanner", "ip cloudflare", "stamp"]);
  expect(screen.getByText("LOGS PAGE")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("tab", { name: "lookup" }));
  expect(screen.getByText("LOOKUP PAGE")).toBeInTheDocument();
});

test("links to logs and fake sni land on the right tab", () => {
  render(<FullView />);
  fireEvent.click(screen.getByRole("button", { name: "dns server" }));
  act(() => useGhost.getState().setPage("fakesni"));
  expect(screen.getByText("FAKESNI PAGE")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "proxy" })).toHaveAttribute("aria-current", "page");
  act(() => useGhost.getState().setPage("logs"));
  expect(screen.getByText("LOGS PAGE")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "công cụ" })).toHaveAttribute("aria-current", "page");
});

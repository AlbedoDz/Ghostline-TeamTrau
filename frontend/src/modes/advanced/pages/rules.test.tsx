import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { Rules } from "./Rules";
import { useGhost } from "../../../app/store";
import { initI18n } from "../../../i18n";

const rule = (pattern: string, extra: Record<string, unknown> = {}) => ({ pattern, enabled: true, ...extra });

const view = {
  rules: [rule("ads.com", { block: true }), rule("youtube.com", { fragment: "on" }), rule("x.com", { sni: "y.com" })],
  text: "ads.com block\nyoutube.com fragment=on\nx.com sni=y.com\n",
  lists: [
    {
      id: "hagezi-light", name: "HaGeZi Light", source: "url", url: "https://github.com/u/r/blob/main/x.txt", format: "auto",
      action: "block", enabled: true, updateHours: 24, lastUpdated: "2026-10-04T10:00:00Z", detected: "domains",
      counts: { domain: 120 }, skipped: 2, skippedSamples: ["not a domain", "bad line"], lastError: "",
    },
  ],
};

const svc = vi.hoisted(() => ({
  GetRules: vi.fn(),
  SaveRulesTable: vi.fn(() => Promise.resolve([])),
  SaveRulesText: vi.fn(() => Promise.resolve([])),
  AddList: vi.fn(() => Promise.resolve({})),
  UpdateList: vi.fn(() => Promise.resolve()),
  DeleteList: vi.fn(() => Promise.resolve()),
  MoveList: vi.fn(() => Promise.resolve()),
  RefreshList: vi.fn(() => Promise.resolve()),
  Catalog: vi.fn(() =>
    Promise.resolve([
      { id: "hostsvn", name: "hostsVN", description: "VN ads", repo: "https://github.com/bigdargon/hostsVN", license: "MIT",
        url: "https://raw.githubusercontent.com/bigdargon/hostsVN/master/hosts", format: "hosts", action: "block" },
    ]),
  ),
  Explain: vi.fn(() => Promise.resolve({ block: true, source: { kind: "list", index: 0, listId: "hagezi-light", line: 120 } })),
  SaveSettings: vi.fn(() => Promise.resolve()),
}));
vi.mock("../../../app/api", () => ({ Service: svc }));

const settings = {
  version: 2, language: "vi", proxy: { enabled: false, port: 8080, upstreams: [], fragment: { mode: "auto" } }, dnsBlockMode: "zero",
};

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  svc.GetRules.mockResolvedValue(structuredClone(view) as any);
  useGhost.getState().reset();
  useGhost.getState().setSettings(structuredClone(settings) as any);
});

test("text tab: errors mark the line and keep the table; success switches back", async () => {
  render(<Rules />);
  await screen.findByText("ads.com");
  fireEvent.click(screen.getByRole("tab", { name: "text" }));
  const box = screen.getByRole("textbox", { name: "rules text" });
  fireEvent.change(box, { target: { value: "ads.com block\nbad.com fragment=maybe\n" } });
  svc.SaveRulesText.mockResolvedValueOnce([{ line: 2, msg: "fragment must be auto, on or off" }] as any);
  fireEvent.click(screen.getByRole("button", { name: "lưu" }));
  expect(await screen.findByText(/dòng 2: fragment must be auto, on or off/)).toBeInTheDocument();
  expect(screen.getByTestId("line-2")).toHaveAttribute("data-error", "true");

  svc.SaveRulesText.mockResolvedValueOnce([] as any);
  svc.GetRules.mockResolvedValueOnce({ ...structuredClone(view), rules: [rule("ads.com", { block: true }), rule("new.com", { block: true })] } as any);
  fireEvent.change(box, { target: { value: "ads.com block\nnew.com block\n" } });
  fireEvent.click(screen.getByRole("button", { name: "lưu" }));
  fireEvent.click(await screen.findByRole("tab", { name: "bảng" }));
  expect(await screen.findByText("new.com")).toBeInTheDocument();
});

test("table: disabling a rule shows #! in the text tab", async () => {
  render(<Rules />);
  await screen.findByText("ads.com");
  fireEvent.click(screen.getByRole("switch", { name: "bật ads.com" }));
  await waitFor(() => expect(svc.SaveRulesTable).toHaveBeenCalled());
  const saved = (svc.SaveRulesTable.mock.calls[0] as any[])[0];
  expect(saved[0].enabled).toBe(false);
  fireEvent.click(screen.getByRole("tab", { name: "text" }));
  expect((screen.getByRole("textbox", { name: "rules text" }) as HTMLTextAreaElement).value).toContain("#! ads.com block");
});

test("table: move buttons reorder rules", async () => {
  render(<Rules />);
  await screen.findByText("ads.com");
  fireEvent.click(screen.getByRole("button", { name: "lên youtube.com" }));
  await waitFor(() => expect(svc.SaveRulesTable).toHaveBeenCalled());
  const saved = (svc.SaveRulesTable.mock.calls[0] as any[])[0];
  expect(saved.map((r: any) => r.pattern)).toEqual(["youtube.com", "ads.com", "x.com"]);
});

test("rules using sni show their fake SNI", async () => {
  render(<Rules />);
  expect(await screen.findByText(/sni=/)).toBeInTheDocument();
  expect(screen.queryByText(/giai đoạn 2B/)).not.toBeInTheDocument();
});

test("lists: add from URL and show detection results", async () => {
  render(<Rules />);
  const row = (await screen.findByText("HaGeZi Light")).closest("tr")!;
  expect(within(row).getByText("domains")).toBeInTheDocument();
  expect(within(row).getByText("120")).toBeInTheDocument();
  fireEvent.click(within(row).getByRole("button", { name: "2 dòng bỏ qua" }));
  expect(await screen.findByText("not a domain")).toBeInTheDocument();

  fireEvent.change(screen.getByRole("textbox", { name: "link danh sách" }), { target: { value: "github.com/u/r/blob/main/x.txt" } });
  fireEvent.click(screen.getByRole("button", { name: "+ thêm danh sách" }));
  await waitFor(() => expect(svc.AddList).toHaveBeenCalled());
  const added = (svc.AddList.mock.calls[0] as any[])[0];
  expect(added.url).toBe("github.com/u/r/blob/main/x.txt");
  expect(added.source).toBe("url");
  expect(added.action).toBe("block");
});

test("quick add shows license and repo and adds with the suggested action", async () => {
  render(<Rules />);
  fireEvent.click(await screen.findByRole("button", { name: "thêm nhanh" }));
  expect(await screen.findByText("MIT")).toBeInTheDocument();
  expect(screen.getByRole("link", { name: /bigdargon\/hostsVN/ })).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "thêm hostsVN" }));
  await waitFor(() => expect(svc.AddList).toHaveBeenCalled());
  expect((svc.AddList.mock.calls[0] as any[])[0]).toMatchObject({ name: "hostsVN", source: "url", action: "block", format: "hosts" });
});

test("domain tester explains the decision", async () => {
  render(<Rules />);
  await screen.findByText("ads.com");
  fireEvent.change(screen.getByRole("textbox", { name: "thử tên miền" }), { target: { value: "x.ads.com" } });
  fireEvent.click(screen.getByRole("button", { name: "thử" }));
  expect(svc.Explain).toHaveBeenCalledWith("x.ads.com");
  expect(await screen.findByText("chặn — danh sách HaGeZi Light, dòng 120")).toBeInTheDocument();
});

const catalog3 = [
  { id: "hostsvn", name: "hostsVN", description: "VN ads", category: "vietnam", repo: "https://github.com/bigdargon/hostsVN", license: "MIT",
    url: "https://raw.githubusercontent.com/bigdargon/hostsVN/master/hosts", format: "hosts", action: "block" },
  { id: "urlhaus", name: "URLhaus (abuse.ch)", description: "malware hosts", category: "security", repo: "https://urlhaus.abuse.ch", license: "CC0-1.0",
    url: "https://urlhaus.abuse.ch/downloads/hostfile/", format: "hosts", action: "block" },
  { id: "v2fly-reddit", name: "Reddit", description: "rd", category: "bypass", repo: "https://github.com/v2fly/domain-list-community", license: "MIT",
    url: "https://raw.githubusercontent.com/v2fly/domain-list-community/master/data/reddit", format: "v2fly", action: "fragment=on" },
];

test("quick add filters by group and search, and marks lists already added", async () => {
  svc.Catalog.mockResolvedValueOnce(catalog3 as any);
  svc.GetRules.mockResolvedValue({ ...structuredClone(view), lists: [...view.lists, { ...view.lists[0], id: "rd", name: "Reddit", url: catalog3[2].url }] } as any);
  render(<Rules />);
  fireEvent.click(await screen.findByRole("button", { name: "thêm nhanh" }));
  expect(await screen.findByText("hostsVN")).toBeInTheDocument();
  // Translated description, not the English fallback.
  expect(screen.getByText(/quảng cáo và tracker hay gặp trên các trang Việt Nam/)).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", { name: "bảo mật" }));
  expect(screen.queryByText("hostsVN")).not.toBeInTheDocument();
  expect(screen.getByText("URLhaus (abuse.ch)")).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", { name: "tất cả" }));
  fireEvent.change(screen.getByRole("textbox", { name: "tìm danh sách" }), { target: { value: "redd" } });
  const panel = screen.getByTestId("catalog");
  expect(within(panel).queryByText("URLhaus (abuse.ch)")).not.toBeInTheDocument();
  expect(within(panel).getByText("Reddit")).toBeInTheDocument();
  expect(within(panel).getByText("đã thêm")).toBeInTheDocument();
  expect(within(panel).queryByRole("button", { name: "thêm Reddit" })).not.toBeInTheDocument();
});

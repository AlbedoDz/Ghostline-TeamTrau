import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Tools } from "./Tools";
import { useGhost } from "../../../../app/store";
import { initI18n } from "../../../../i18n";

const svc = vi.hoisted(() => ({
  LookupTypes: vi.fn(() => Promise.resolve(["A", "AAAA", "MX", "PTR"])),
  DefaultLookupSources: vi.fn(() =>
    Promise.resolve([
      { kind: "ghostline", ref: "", label: "Ghostline" },
      { kind: "server", ref: "cf", label: "Cloudflare" },
    ]),
  ),
  ISPResolvers: vi.fn(() => Promise.resolve(["203.162.4.191"])),
  Lookup: vi.fn(),
  DecodeStamps: vi.fn(() => Promise.resolve([])),
}));
vi.mock("../../../../app/api", () => ({ Service: svc }));

const answer = (label: string, ips: string[], extra: Record<string, unknown> = {}) => ({
  source: { kind: "server", ref: label, label }, ok: true, rcode: "NOERROR", ad: false, tc: false, ra: true, latencyMs: 12,
  records: ips.map((ip) => ({ name: "youtube.com.", type: "A", ttl: 300, data: ip })),
  dig: `;; ANSWER SECTION:\nyoutube.com. 300 IN A ${ips[0] ?? ""}\n;; SERVER: ${label}`, ...extra,
});

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  useGhost.getState().reset();
  useGhost.getState().setPage("tools");
  useGhost.getState().setToolsTab("lookup");
});

test("the ISP source is offered unchecked and marked unencrypted", async () => {
  render(<Tools />);
  const isp = await screen.findByRole("checkbox", { name: /203\.162\.4\.191/ });
  expect(isp).not.toBeChecked();
  expect(screen.getByText(/⚠ không mã hoá/)).toBeInTheDocument();
  expect(screen.getByRole("checkbox", { name: /Cloudflare/ })).toBeChecked();
});

test("a lookup sends the checked sources and shows the poisoned verdict", async () => {
  svc.Lookup.mockResolvedValueOnce({
    overall: "poisoned", verdicts: ["match", "poisoned"],
    answers: [answer("Cloudflare", ["142.250.1.1"]), answer("203.162.4.191", ["10.10.34.35"])],
  });
  render(<Tools />);
  fireEvent.click(await screen.findByRole("checkbox", { name: /203\.162\.4\.191/ }));
  fireEvent.click(screen.getByRole("checkbox", { name: /Ghostline/ }));
  fireEvent.change(screen.getByRole("textbox", { name: "tên miền" }), { target: { value: "youtube.com" } });
  fireEvent.click(screen.getByRole("button", { name: "tra" }));
  await waitFor(() => expect(svc.Lookup).toHaveBeenCalled());
  const [name, qtype, sources] = svc.Lookup.mock.calls[0] as any[];
  expect(name).toBe("youtube.com");
  expect(qtype).toBe("A");
  expect(sources).toEqual([{ kind: "server", ref: "cf", label: "Cloudflare" }, { kind: "isp", ref: "203.162.4.191", label: "203.162.4.191" }]);
  expect(await screen.findByText("Bị đầu độc DNS")).toBeInTheDocument();
  expect(screen.getByText("10.10.34.35")).toBeInTheDocument();
});

test.each([
  ["differs", "Kết quả khác nhau"],
  ["match", "Kết quả khớp nhau"],
])("verdict %s has its own title", async (overall, title) => {
  svc.Lookup.mockResolvedValueOnce({ overall, verdicts: ["match"], answers: [answer("Cloudflare", ["1.1.1.1"])] });
  render(<Tools />);
  fireEvent.change(await screen.findByRole("textbox", { name: "tên miền" }), { target: { value: "x.com" } });
  fireEvent.click(screen.getByRole("button", { name: "tra" }));
  expect(await screen.findByText(title)).toBeInTheDocument();
});

test("details toggle shows the dig text", async () => {
  svc.Lookup.mockResolvedValueOnce({ overall: "match", verdicts: ["match"], answers: [answer("Cloudflare", ["1.1.1.1"])] });
  render(<Tools />);
  fireEvent.change(await screen.findByRole("textbox", { name: "tên miền" }), { target: { value: "x.com" } });
  fireEvent.click(screen.getByRole("button", { name: "tra" }));
  await screen.findByText("Kết quả khớp nhau");
  expect(screen.queryByText(/ANSWER SECTION/)).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", { name: "chi tiết" }));
  expect(screen.getByText(/ANSWER SECTION/)).toBeInTheDocument();
});

test("LOOKUP_NOT_CONNECTED shows the connect hint", async () => {
  svc.Lookup.mockRejectedValueOnce(new Error("LOOKUP_NOT_CONNECTED"));
  render(<Tools />);
  fireEvent.change(await screen.findByRole("textbox", { name: "tên miền" }), { target: { value: "x.com" } });
  fireEvent.click(screen.getByRole("button", { name: "tra" }));
  expect(await screen.findByText(/Ghostline chưa kết nối/)).toBeInTheDocument();
});

test("the open tab survives leaving the page", async () => {
  const { unmount } = render(<Tools />);
  fireEvent.click(screen.getByRole("tab", { name: "stamp" }));
  unmount();
  render(<Tools />);
  expect(screen.getByRole("tab", { name: "stamp" })).toHaveAttribute("aria-selected", "true");
});

test("while a lookup runs, the button and a status line say so", async () => {
  let finish: (v: any) => void = () => {};
  svc.Lookup.mockImplementationOnce(() => new Promise((r) => (finish = r)));
  render(<Tools />);
  fireEvent.change(await screen.findByRole("textbox", { name: "tên miền" }), { target: { value: "x.com" } });
  fireEvent.click(screen.getByRole("button", { name: "tra" }));
  const busy = await screen.findByRole("button", { name: "đang tra…" });
  expect(busy).toBeDisabled();
  expect(screen.getByRole("status")).toHaveTextContent("Đang tra x.com qua 2 nguồn");
  finish({ overall: "match", verdicts: ["match"], answers: [answer("Cloudflare", ["1.1.1.1"])] });
  expect(await screen.findByText("Kết quả khớp nhau")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "tra" })).toBeEnabled();
  expect(screen.queryByRole("status")).toBeNull();
});

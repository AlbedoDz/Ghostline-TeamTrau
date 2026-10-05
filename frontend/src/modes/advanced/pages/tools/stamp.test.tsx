import { beforeAll, beforeEach, expect, test, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { Stamp } from "./Stamp";
import { useGhost } from "../../../../app/store";
import { initI18n } from "../../../../i18n";

const svc = vi.hoisted(() => ({
  DecodeStamps: vi.fn(),
  EncodeStamp: vi.fn(),
  StampFromURL: vi.fn(),
  AddServers: vi.fn(() => Promise.resolve([1, []])),
}));
vi.mock("../../../../app/api", () => ({ Service: svc }));

const doh = {
  proto: "doh", addr: "217.169.20.22", host: "dns.aa.net.uk", path: "/dns-query", providerName: "", publicKey: "",
  hashes: null, dnssec: true, noLog: true, noFilter: true, stamp: "sdns://AgcAAA", usable: true,
};

beforeAll(() => initI18n("vi"));
beforeEach(() => {
  vi.clearAllMocks();
  useGhost.getState().reset();
});

test("decoding shows one card per line, with errors", async () => {
  svc.DecodeStamps.mockResolvedValueOnce([
    { line: "sdns://AgcAAA", fields: doh },
    { line: "nope", error: "stamps: invalid: stamp: must start with sdns://" },
    { line: "sdns://gQ", fields: { ...doh, proto: "dnscrypt-relay", usable: false, host: "", addr: "1.2.3.4:443" } },
  ]);
  render(<Stamp />);
  fireEvent.change(screen.getByRole("textbox", { name: "stamp cần giải mã" }), { target: { value: "sdns://AgcAAA\nnope\nsdns://gQ" } });
  fireEvent.click(screen.getByRole("button", { name: "giải mã" }));
  expect(await screen.findByText("dns.aa.net.uk")).toBeInTheDocument();
  expect(screen.getByText(/must start with sdns/)).toBeInTheDocument();
  expect(screen.getByText("Ghostline không dùng loại này")).toBeInTheDocument();
  expect(screen.getAllByRole("button", { name: "thêm vào danh sách server" })).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "thêm vào danh sách server" }));
  await waitFor(() => expect(svc.AddServers).toHaveBeenCalledWith("sdns://AgcAAA"));
  expect(await screen.findByText("Đã thêm 1 máy chủ")).toBeInTheDocument();
});

test("building from a URL fills the form, then makes the stamp", async () => {
  svc.StampFromURL.mockResolvedValueOnce({ ...doh, hashes: [], stamp: "", usable: false, dnssec: false, noLog: false, noFilter: false, addr: "8.8.8.8", host: "dns.google" });
  svc.EncodeStamp.mockResolvedValueOnce("sdns://AgAAAAAAAAAABzguOC44Ljg");
  render(<Stamp />);
  fireEvent.change(screen.getByRole("textbox", { name: "URL" }), { target: { value: "https://dns.google/dns-query" } });
  fireEvent.change(screen.getByRole("textbox", { name: "IP (tuỳ chọn)" }), { target: { value: "8.8.8.8" } });
  fireEvent.click(screen.getByRole("button", { name: "điền từ URL" }));
  await waitFor(() => expect(svc.StampFromURL).toHaveBeenCalledWith("https://dns.google/dns-query", "8.8.8.8"));
  expect(await screen.findByDisplayValue("dns.google")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("checkbox", { name: "DNSSEC" }));
  fireEvent.click(screen.getByRole("button", { name: "tạo stamp" }));
  await waitFor(() => expect(svc.EncodeStamp).toHaveBeenCalled());
  const f = (svc.EncodeStamp.mock.calls[0] as any[])[0];
  expect(f.host).toBe("dns.google");
  expect(f.dnssec).toBe(true);
  expect(await screen.findByText("sdns://AgAAAAAAAAAABzguOC44Ljg")).toBeInTheDocument();
});

test("an invalid field shows the STAMP_INVALID message with the field", async () => {
  svc.EncodeStamp.mockRejectedValueOnce(new Error("STAMP_INVALID: stamps: invalid: hashes: each must be 32 bytes in hex"));
  render(<Stamp />);
  fireEvent.change(screen.getByRole("textbox", { name: "host" }), { target: { value: "a.com" } });
  fireEvent.change(screen.getByRole("textbox", { name: "hash chứng chỉ" }), { target: { value: "abcd" } });
  fireEvent.click(screen.getByRole("button", { name: "tạo stamp" }));
  expect(await screen.findByText(/Stamp không hợp lệ.*hashes/)).toBeInTheDocument();
  expect((svc.EncodeStamp.mock.calls[0] as any[])[0].hashes).toEqual(["abcd"]);
});

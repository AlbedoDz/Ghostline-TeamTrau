import { expect, test, vi } from "vitest";
import { fireEvent, render, screen, within } from "@testing-library/react";
import { PowerButton } from "./PowerButton";
import { Toggle } from "./Toggle";
import { DataTable } from "./DataTable";
import { Sparkline } from "./Sparkline";
import { Banner } from "./Banner";
import { Chip } from "./Chip";
import { TerminalPanel } from "./TerminalPanel";
import { Sidebar } from "./Sidebar";

test("PowerButton exposes state via data attribute and aria-label", () => {
  const onClick = vi.fn();
  render(<PowerButton state="busy" label="Đang kết nối" onClick={onClick} />);
  const b = screen.getByRole("button", { name: "Đang kết nối" });
  expect(b).toHaveAttribute("data-state", "busy");
  fireEvent.click(b);
  expect(onClick).toHaveBeenCalled();
});

test("Toggle is a switch reflecting checked", () => {
  const onChange = vi.fn();
  render(<Toggle checked={false} onChange={onChange} label="vượt dpi" />);
  const sw = screen.getByRole("switch", { name: "vượt dpi" });
  expect(sw).toHaveAttribute("aria-checked", "false");
  fireEvent.click(sw);
  expect(onChange).toHaveBeenCalledWith(true);
});

type Row = { id: string; name: string; ms: number };
const rows: Row[] = [
  { id: "a", name: "alpha", ms: 30 },
  { id: "b", name: "bravo", ms: 10 },
  { id: "c", name: "charlie", ms: 20 },
];

test("DataTable sorts by clicked column and toggles direction", () => {
  render(
    <DataTable<Row>
      rows={rows}
      rowKey={(r) => r.id}
      columns={[
        { key: "name", label: "tên", render: (r) => r.name, sort: (a, b) => a.name.localeCompare(b.name) },
        { key: "ms", label: "độ trễ", render: (r) => String(r.ms), sort: (a, b) => a.ms - b.ms },
      ]}
      initialSort={{ key: "ms", dir: "asc" }}
    />,
  );
  const names = () => screen.getAllByRole("row").slice(1).map((r) => within(r).getAllByRole("cell")[0].textContent);
  expect(names()).toEqual(["bravo", "charlie", "alpha"]);
  fireEvent.click(screen.getByRole("columnheader", { name: /độ trễ/ }));
  expect(names()).toEqual(["alpha", "charlie", "bravo"]);
  fireEvent.click(screen.getByRole("columnheader", { name: /tên/ }));
  expect(names()).toEqual(["alpha", "bravo", "charlie"]);
});

test("Sparkline renders polyline with N points", () => {
  const { container } = render(<Sparkline points={[1, 2, 3]} width={300} height={56} />);
  expect(container.querySelector("polyline")!.getAttribute("points")!.trim().split(" ")).toHaveLength(3);
});

test("Sparkline with no data renders nothing broken", () => {
  const { container } = render(<Sparkline points={[]} width={300} height={56} />);
  expect(container.querySelector("polyline")).toBeNull();
});

test("Banner renders actions as buttons", () => {
  const a = vi.fn();
  render(
    <Banner tone="warn" actions={[{ label: "TỰ DÒ", onClick: a, primary: true }, { label: "bỏ qua", onClick: () => {} }]}>
      2/4 trang mẫu
    </Banner>,
  );
  expect(screen.getByRole("alert")).toHaveTextContent("2/4 trang mẫu");
  fireEvent.click(screen.getByRole("button", { name: "TỰ DÒ" }));
  expect(a).toHaveBeenCalled();
});

test("Chip reflects active as aria-pressed", () => {
  render(<Chip active onClick={() => {}}>doh</Chip>);
  expect(screen.getByRole("button", { name: "doh" })).toHaveAttribute("aria-pressed", "true");
});

test("TerminalPanel renders key/value rows", () => {
  render(<TerminalPanel rows={[{ k: "máy chủ", v: "cloudflare" }, { k: "độ trễ", v: "24 ms", tone: "ok" }]} />);
  expect(screen.getByText("máy chủ")).toBeInTheDocument();
  expect(screen.getByText("24 ms")).toHaveAttribute("data-tone", "ok");
});

test("Sidebar marks the active item and selects", () => {
  const onSelect = vi.fn();
  render(<Sidebar items={[{ id: "a", label: "tổng quan" }, { id: "b", label: "máy chủ" }]} active="b" onSelect={onSelect} footer={<span>foot</span>} />);
  expect(screen.getByRole("button", { name: "máy chủ" })).toHaveAttribute("aria-current", "page");
  fireEvent.click(screen.getByRole("button", { name: "tổng quan" }));
  expect(onSelect).toHaveBeenCalledWith("a");
  expect(screen.getByText("foot")).toBeInTheDocument();
});

test("Sidebar shows group headers that are not buttons", () => {
  render(<Sidebar items={[{ id: "h1", label: "cơ bản", header: true }, { id: "a", label: "tổng quan" }]} active="a" onSelect={() => {}} />);
  expect(screen.getByText("cơ bản")).toBeInTheDocument();
  expect(screen.queryByRole("button", { name: "cơ bản" })).toBeNull();
  expect(screen.getAllByRole("button")).toHaveLength(1);
});

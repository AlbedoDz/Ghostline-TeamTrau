import { beforeAll, expect, test, vi } from "vitest";
import { fireEvent, render, screen } from "@testing-library/react";
import { TitleBar } from "./TitleBar";
import { initI18n } from "../../i18n";

beforeAll(() => initI18n("vi"));

test("language toggle calls onLang with the other language", () => {
  const onLang = vi.fn();
  render(<TitleBar mode="simple" onMode={() => {}} lang="vi" onLang={onLang} />);
  fireEvent.click(screen.getByRole("button", { name: /EN/ }));
  expect(onLang).toHaveBeenCalledWith("en");
});

test("the mode switch is a two-way control in the title bar", () => {
  const onMode = vi.fn();
  const { container } = render(<TitleBar mode="simple" onMode={onMode} lang="vi" onLang={() => {}} />);
  const group = screen.getByRole("radiogroup", { name: "giao diện" });
  expect(group.getAttribute("title")).toMatch(/Đơn giản: chỉ có nút kết nối/);
  expect(screen.getByRole("radio", { name: "Đơn giản" })).toHaveAttribute("aria-checked", "true");
  fireEvent.click(screen.getByRole("radio", { name: "Đầy đủ" }));
  expect(onMode).toHaveBeenCalledWith("full");
  expect(container.querySelector("[data-drag]")!.contains(group), "it sits in the title bar").toBe(true);
  expect(container.querySelector('[role="tablist"]'), "no separate mode row").toBeNull();
});

test("drag region and no-drag buttons", () => {
  const { container } = render(<TitleBar mode="simple" onMode={() => {}} lang="vi" onLang={() => {}} />);
  const bar = container.querySelector("[data-drag]") as HTMLElement;
  expect(bar.style.getPropertyValue("--wails-draggable")).toBe("drag");
  for (const b of Array.from(bar.querySelectorAll("button"))) {
    expect((b as HTMLElement).style.getPropertyValue("--wails-draggable")).toBe("no-drag");
  }
});

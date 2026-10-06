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

test("mode tabs sit on their own row and call onMode", () => {
  const onMode = vi.fn();
  const { container } = render(<TitleBar mode="simple" onMode={onMode} lang="vi" onLang={() => {}} />);
  const simple = screen.getByRole("tab", { name: "[ĐƠN GIẢN]" });
  expect(simple).toHaveAttribute("aria-selected", "true");
  fireEvent.click(screen.getByRole("tab", { name: "ĐẦY ĐỦ" }));
  expect(onMode).toHaveBeenCalledWith("full");
  expect(container.querySelector("[data-drag]")!.contains(simple), "below the title bar, not in it").toBe(false);
});

test("drag region and no-drag buttons", () => {
  const { container } = render(<TitleBar mode="simple" onMode={() => {}} lang="vi" onLang={() => {}} />);
  const bar = container.querySelector("[data-drag]") as HTMLElement;
  expect(bar.style.getPropertyValue("--wails-draggable")).toBe("drag");
  for (const b of Array.from(bar.querySelectorAll("button"))) {
    expect((b as HTMLElement).style.getPropertyValue("--wails-draggable")).toBe("no-drag");
  }
});

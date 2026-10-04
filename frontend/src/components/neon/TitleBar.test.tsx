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

test("mode tabs call onMode", () => {
  const onMode = vi.fn();
  render(<TitleBar mode="simple" onMode={onMode} lang="vi" onLang={() => {}} />);
  fireEvent.click(screen.getByRole("tab", { name: /NÂNG CAO/ }));
  expect(onMode).toHaveBeenCalledWith("advanced");
  expect(screen.getByRole("tab", { name: /ĐƠN GIẢN/ })).toHaveAttribute("aria-selected", "true");
});

test("drag region and no-drag buttons", () => {
  const { container } = render(<TitleBar mode="simple" onMode={() => {}} lang="vi" onLang={() => {}} />);
  const bar = container.querySelector("[data-drag]") as HTMLElement;
  expect(bar.style.getPropertyValue("--wails-draggable")).toBe("drag");
  for (const b of Array.from(bar.querySelectorAll("button"))) {
    expect((b as HTMLElement).style.getPropertyValue("--wails-draggable")).toBe("no-drag");
  }
});

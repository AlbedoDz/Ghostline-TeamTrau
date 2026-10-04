import { useEffect, useRef, useState } from "react";
import css from "./neon.module.css";

export type MenuItem = { label: string; onSelect: () => void };

type Props = { x: number; y: number; items: MenuItem[]; onClose: () => void };

/** ContextMenu is an in-app right-click menu: arrows move, Enter runs, Esc or a click outside closes. */
export function ContextMenu({ x, y, items, onClose }: Props) {
  const ref = useRef<HTMLDivElement>(null);
  const [active, setActive] = useState(0);

  useEffect(() => {
    ref.current?.focus();
    const outside = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) onClose();
    };
    document.addEventListener("mousedown", outside);
    return () => document.removeEventListener("mousedown", outside);
  }, [onClose]);

  const run = (i: number) => {
    onClose();
    items[i]?.onSelect();
  };

  return (
    <div
      ref={ref}
      role="menu"
      tabIndex={-1}
      className={css.menu}
      style={{ left: Math.min(x, window.innerWidth - 240), top: Math.min(y, window.innerHeight - items.length * 30 - 12) }}
      onContextMenu={(e) => e.preventDefault()}
      onKeyDown={(e) => {
        if (e.key === "Escape") onClose();
        else if (e.key === "ArrowDown") setActive((a) => (a + 1) % items.length);
        else if (e.key === "ArrowUp") setActive((a) => (a - 1 + items.length) % items.length);
        else if (e.key === "Enter") run(active);
        else return;
        e.preventDefault();
      }}
    >
      {items.map((it, i) => (
        <button key={it.label} role="menuitem" className={css.menuItem} data-active={i === active} onMouseEnter={() => setActive(i)} onClick={() => run(i)}>
          {it.label}
        </button>
      ))}
    </div>
  );
}

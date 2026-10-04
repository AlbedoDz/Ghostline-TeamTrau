import type { ReactNode } from "react";
import css from "./neon.module.css";

type Props = { items: { id: string; label: string }[]; active: string; onSelect: (id: string) => void; footer?: ReactNode };

export function Sidebar({ items, active, onSelect, footer }: Props) {
  return (
    <nav className={css.side}>
      {items.map((it) => (
        <button key={it.id} aria-current={it.id === active ? "page" : undefined} onClick={() => onSelect(it.id)}>
          {it.label}
        </button>
      ))}
      {footer && <div className={css.foot}>{footer}</div>}
    </nav>
  );
}

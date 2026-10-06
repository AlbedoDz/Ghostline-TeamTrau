import type { ReactNode } from "react";
import css from "./neon.module.css";

// A header item is a group label, not a page; an empty label is a divider.
type Item = { id: string; label: string; header?: boolean };
type Props = { items: Item[]; active: string; onSelect: (id: string) => void; footer?: ReactNode };

export function Sidebar({ items, active, onSelect, footer }: Props) {
  return (
    <nav className={css.side}>
      {items.map((it) =>
        it.header ? (
          it.label ? (
            <div key={it.id} className={css.sideHeader}>{it.label}</div>
          ) : (
            <div key={it.id} role="separator" className={css.sideDivider} />
          )
        ) : (
          <button key={it.id} aria-current={it.id === active ? "page" : undefined} onClick={() => onSelect(it.id)}>
            {it.label}
          </button>
        ),
      )}
      {footer && <div className={css.foot}>{footer}</div>}
    </nav>
  );
}

import type { ReactNode } from "react";
import css from "./neon.module.css";

type Props = { active?: boolean; onClick?: () => void; children: ReactNode; title?: string; label?: string };

export function Chip({ active, onClick, children, title, label }: Props) {
  return (
    <button className={css.chip} aria-pressed={!!active} onClick={onClick} title={title} aria-label={label}>
      {children}
    </button>
  );
}

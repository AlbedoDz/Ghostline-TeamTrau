import type { ReactNode } from "react";
import css from "./neon.module.css";

type Props = { active?: boolean; onClick?: () => void; children: ReactNode; title?: string };

export function Chip({ active, onClick, children, title }: Props) {
  return (
    <button className={css.chip} aria-pressed={!!active} onClick={onClick} title={title}>
      {children}
    </button>
  );
}

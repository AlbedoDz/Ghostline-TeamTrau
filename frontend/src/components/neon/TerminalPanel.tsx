import type { ReactNode } from "react";
import css from "./neon.module.css";

export type Tone = "ok" | "dim" | "warn" | "err";
export type TermRow = { k: string; v: ReactNode; tone?: Tone };

type Props = { rows?: TermRow[]; lines?: ReactNode[]; className?: string };

export function TerminalPanel({ rows, lines, className }: Props) {
  return (
    <div className={`${css.term} ${className ?? ""}`}>
      {rows?.map((r) => (
        <div key={r.k} className={css.row}>
          <span className={css.k}>{r.k}</span>
          <span className={css.v} data-tone={r.tone}>
            {r.v}
          </span>
        </div>
      ))}
      {lines?.map((l, i) => (
        <div key={i}>{l}</div>
      ))}
    </div>
  );
}

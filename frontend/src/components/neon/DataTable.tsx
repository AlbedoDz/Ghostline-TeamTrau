import { useMemo, useState, type ReactNode } from "react";
import css from "./neon.module.css";

export type Column<T> = {
  key: string;
  label: string;
  render: (row: T) => ReactNode;
  sort?: (a: T, b: T) => number;
};

type Sort = { key: string; dir: "asc" | "desc" };

type Props<T> = {
  columns: Column<T>[];
  rows: T[];
  rowKey: (row: T) => string;
  initialSort?: Sort;
  highlight?: (row: T) => boolean;
};

export function DataTable<T>({ columns, rows, rowKey, initialSort, highlight }: Props<T>) {
  const [sort, setSort] = useState<Sort | undefined>(initialSort);
  const sorted = useMemo(() => {
    const col = columns.find((c) => c.key === sort?.key);
    if (!col?.sort) return rows;
    const out = [...rows].sort(col.sort);
    return sort!.dir === "desc" ? out.reverse() : out;
  }, [rows, columns, sort]);

  const onHeader = (c: Column<T>) => {
    if (!c.sort) return;
    setSort((s) => (s?.key === c.key ? { key: c.key, dir: s.dir === "asc" ? "desc" : "asc" } : { key: c.key, dir: "asc" }));
  };

  return (
    <table className={css.table}>
      <thead>
        <tr>
          {columns.map((c) => (
            <th
              key={c.key}
              onClick={() => onHeader(c)}
              aria-sort={sort?.key === c.key ? (sort.dir === "asc" ? "ascending" : "descending") : undefined}
            >
              {c.label}
              {sort?.key === c.key ? (sort.dir === "asc" ? " ▲" : " ▼") : ""}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {sorted.map((r) => (
          <tr key={rowKey(r)} data-highlight={highlight?.(r) ?? false}>
            {columns.map((c) => (
              <td key={c.key}>{c.render(r)}</td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

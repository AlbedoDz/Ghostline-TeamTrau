import { useMemo, useState, type ReactNode } from "react";
import css from "./neon.module.css";

export type Column<T> = {
  key: string;
  label: string;
  render: (row: T) => ReactNode;
  sort?: (a: T, b: T) => number;
  align?: "left" | "right" | "center";
  width?: string; // CSS width for a fixed, balanced layout
};

type Sort = { key: string; dir: "asc" | "desc" };

type Props<T> = {
  columns: Column<T>[];
  rows: T[];
  rowKey: (row: T) => string;
  initialSort?: Sort;
  highlight?: (row: T) => boolean;
  // pinTop keeps matching rows above the rest, whatever the sort.
  pinTop?: (row: T) => boolean;
  // onRowMenu opens a row's context menu (right click, Menu key, Shift+F10).
  onRowMenu?: (row: T, x: number, y: number) => void;
  // onRowDoubleClick runs on a left double-click (not on buttons inside the row).
  onRowDoubleClick?: (row: T) => void;
};

export function DataTable<T>({ columns, rows, rowKey, initialSort, highlight, pinTop, onRowMenu, onRowDoubleClick }: Props<T>) {
  const [sort, setSort] = useState<Sort | undefined>(initialSort);
  const sorted = useMemo(() => {
    const col = columns.find((c) => c.key === sort?.key);
    let out = rows;
    if (col?.sort) {
      out = [...rows].sort(col.sort);
      if (sort!.dir === "desc") out.reverse();
    }
    if (!pinTop) return out;
    return [...out.filter(pinTop), ...out.filter((r) => !pinTop(r))];
  }, [rows, columns, sort, pinTop]);

  const onHeader = (c: Column<T>) => {
    if (!c.sort) return;
    setSort((s) => (s?.key === c.key ? { key: c.key, dir: s.dir === "asc" ? "desc" : "asc" } : { key: c.key, dir: "asc" }));
  };

  return (
    <table className={css.table}>
      <colgroup>
        {columns.map((c) => (
          <col key={c.key} style={c.width ? { width: c.width } : undefined} />
        ))}
      </colgroup>
      <thead>
        <tr>
          {columns.map((c) => (
            <th
              key={c.key}
              onClick={() => onHeader(c)}
              style={{ textAlign: c.align ?? "left", cursor: c.sort ? "pointer" : "default" }}
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
          <tr
            key={rowKey(r)}
            data-highlight={highlight?.(r) ?? false}
            tabIndex={onRowMenu ? 0 : undefined}
            onDoubleClick={
              onRowDoubleClick
                ? (e) => {
                    if ((e.target as HTMLElement).closest("button")) return; // the star/remove buttons act on their own
                    window.getSelection()?.removeAllRanges(); // a double-click selects text otherwise
                    onRowDoubleClick(r);
                  }
                : undefined
            }
            onContextMenu={
              onRowMenu
                ? (e) => {
                    e.preventDefault();
                    onRowMenu(r, e.clientX, e.clientY);
                  }
                : undefined
            }
            onKeyDown={
              onRowMenu
                ? (e) => {
                    if (e.key === "ContextMenu" || (e.key === "F10" && e.shiftKey)) {
                      e.preventDefault();
                      const b = e.currentTarget.getBoundingClientRect();
                      onRowMenu(r, b.left + 24, b.bottom);
                    }
                  }
                : undefined
            }
          >
            {columns.map((c) => (
              <td key={c.key} style={{ textAlign: c.align ?? "left" }}>
                {c.render(r)}
              </td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  );
}

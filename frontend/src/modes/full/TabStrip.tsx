import css from "./full.module.css";

type Props<T extends string> = { tabs: { id: T; label: string }[]; active: T; onSelect: (id: T) => void };

/** TabStrip is the row of tabs at the top of a page with sub-pages. */
export function TabStrip<T extends string>({ tabs, active, onSelect }: Props<T>) {
  return (
    <div className={css.tabStrip} role="tablist">
      {tabs.map((tb) => (
        <button key={tb.id} role="tab" aria-selected={active === tb.id} className={css.chipTab} onClick={() => onSelect(tb.id)}>
          {tb.label}
        </button>
      ))}
    </div>
  );
}

import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type CatalogItem, type List } from "../../../app/api";
import { describeError, tCode } from "../../../i18n";
import { Toggle } from "../../../components/neon/Toggle";
import { Chip } from "../../../components/neon/Chip";
import css from "../advanced.module.css";

type Props = { lists: List[]; upstreams: string[]; onChanged: () => void };

const ACTIONS = ["block", "allow", "fragment=on", "fromFile"] as const;
const CATEGORIES = ["ads", "security", "adult", "gambling", "social", "telemetry", "vietnam", "bypass"] as const;

const total = (c?: { [k: string]: number | undefined } | null) => Object.values(c ?? {}).reduce<number>((a, b) => a + (b ?? 0), 0);
const isPath = (s: string) => /^[a-zA-Z]:\\|^\\\\/.test(s);
const repoName = (u: string) => u.replace(/^https:\/\/(github\.com\/)?/, "");

/** visibleCatalog filters the quick-add catalog by group and free text. */
export function visibleCatalog(items: CatalogItem[], category: string, query: string, describe: (c: CatalogItem) => string): CatalogItem[] {
  const q = query.trim().toLowerCase();
  return items.filter(
    (c) =>
      (category === "all" || c.category === category) &&
      (!q || [c.name, describe(c), c.description, c.repo].some((s) => s.toLowerCase().includes(q))),
  );
}

/** Lists manages community lists: add by link/file, quick add, refresh, order. */
export function Lists({ lists, upstreams, onChanged }: Props) {
  const { t } = useTranslation();
  const [link, setLink] = useState("");
  const [action, setAction] = useState<string>("block");
  const [error, setError] = useState<string | null>(null);
  const [samples, setSamples] = useState<string | null>(null);
  const [catalog, setCatalog] = useState<CatalogItem[] | null>(null);
  const [category, setCategory] = useState<string>("all");
  const [query, setQuery] = useState("");

  const run = (p: Promise<unknown>) =>
    p.then(() => { setError(null); onChanged(); }).catch((e) => setError(describeError(e)));

  const add = () => {
    const v = link.trim();
    if (!v) return;
    const name = v.split(/[\\/]/).filter(Boolean).pop() ?? v;
    const l: any = isPath(v) ? { name, source: "file", path: v, action } : { name, source: "url", url: v, action };
    run(Service.AddList(l).then(() => setLink("")));
  };

  // Descriptions are translated by catalog id; the Go text is the fallback.
  const describe = (c: CatalogItem) => {
    const key = `catalog.items.${c.id}`;
    const s = t(key);
    return s === key ? c.description : s;
  };

  const update = (l: List, patch: Partial<List>) => run(Service.UpdateList({ ...l, ...patch } as List));
  const actions = [...ACTIONS, ...upstreams.map((u) => `upstream=${u}`)];

  return (
    <div className={css.panel}>
      <div className={css.panelTitle}>
        <span>{t("rules.lists.title")}</span>
        <span className={css.row}>
          <Chip onClick={() => void Service.Catalog().then((c) => setCatalog(c ?? []))}>{t("rules.lists.quick")}</Chip>
          <Chip onClick={() => run(Service.RefreshList(""))}>{t("rules.lists.refreshAll")}</Chip>
        </span>
      </div>
      <table style={{ width: "100%" }}>
        <tbody>
          {lists.map((l, i) => (
            <tr key={l.id}>
              <td><Toggle label={t("rules.enableFor", { pattern: l.name })} checked={l.enabled} onChange={(v) => update(l, { enabled: v })} /></td>
              <td title={l.url || l.path}>{l.name}</td>
              <td className={css.dim}>{l.detected || "—"}</td>
              <td>{total(l.counts)}</td>
              <td>
                {l.skipped > 0 && (
                  <Chip label={t("rules.lists.skipped", { count: l.skipped })} onClick={() => setSamples(samples === l.id ? null : l.id)}>
                    {t("rules.lists.skipped", { count: l.skipped })}
                  </Chip>
                )}
              </td>
              <td className={css.dim}>{l.lastUpdated && !l.lastUpdated.startsWith("0001") ? new Date(l.lastUpdated).toLocaleString() : "—"}</td>
              <td className={css.bad}>{l.lastError ? tCode(`errors.${l.lastError}.message`, { id: l.name }) : ""}</td>
              <td>
                <span className={css.row}>
                  <Chip label={t("rules.lists.refreshFor", { name: l.name })} onClick={() => run(Service.RefreshList(l.id))}>⟳</Chip>
                  <Chip label={t("rules.up", { pattern: l.name })} onClick={() => run(Service.MoveList(l.id, Math.max(0, i - 1)))}>▲</Chip>
                  <Chip label={t("rules.down", { pattern: l.name })} onClick={() => run(Service.MoveList(l.id, i + 1))}>▼</Chip>
                  <Chip label={t("rules.delete", { pattern: l.name })} onClick={() => run(Service.DeleteList(l.id))}>✕</Chip>
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {lists.length === 0 && <div className={css.dim}>{t("rules.lists.empty")}</div>}
      {samples && (
        <div className={css.code}>
          {(lists.find((l) => l.id === samples)?.skippedSamples ?? []).map((s) => (
            <div key={s}>{s}</div>
          ))}
        </div>
      )}
      <div className={css.row} style={{ marginTop: 8, flexWrap: "wrap" }}>
        <input aria-label={t("rules.lists.link")} placeholder="https://github.com/… · C:\…\list.txt" value={link}
          onChange={(e) => setLink(e.target.value)} style={{ flex: 1, minWidth: 240 }} />
        <select aria-label={t("rules.actionLabel")} value={action} onChange={(e) => setAction(e.target.value)}>
          {actions.map((a) => (
            <option key={a} value={a}>{a.startsWith("upstream=") ? a : t(`rules.lists.actions.${a}`)}</option>
          ))}
        </select>
        <Chip onClick={add}>{t("rules.lists.add")}</Chip>
      </div>
      <div className={css.dim}>{t("rules.lists.formats")}</div>
      {error && <div className={css.bad}>{error}</div>}
      {catalog && (
        <div className={css.panel} style={{ marginTop: 8 }} data-testid="catalog">
          <div className={css.row} style={{ flexWrap: "wrap" }}>
            <Chip active={category === "all"} onClick={() => setCategory("all")}>{t("rules.lists.all")}</Chip>
            {CATEGORIES.filter((c) => catalog.some((i) => i.category === c)).map((c) => (
              <Chip key={c} active={category === c} onClick={() => setCategory(c)}>{t(`catalog.categories.${c}`)}</Chip>
            ))}
            <input aria-label={t("rules.lists.search")} placeholder={t("rules.lists.search")} value={query}
              onChange={(e) => setQuery(e.target.value)} style={{ flex: 1, minWidth: 160 }} />
          </div>
          {visibleCatalog(catalog, category, query, (c) => describe(c)).map((c) => {
            const added = lists.some((l) => l.url === c.url);
            return (
              <div key={c.id} className={css.setting}>
                <span>
                  <b>{c.name}</b> · <span className={css.dim}>{describe(c)}</span>
                  <br />
                  <span className={css.dim}>{t(`catalog.categories.${c.category}`)}</span> ·{" "}
                  <a href={c.repo} target="_blank" rel="noreferrer">{repoName(c.repo)}</a> · <span>{c.license}</span>
                  {c.action === "fragment=on" && <span className={css.dim}> · {t("rules.lists.actions.fragment=on")}</span>}
                </span>
                {added ? (
                  <span className={css.ok}>{t("rules.lists.added")}</span>
                ) : (
                  <Chip label={t("rules.lists.addItem", { name: c.name })}
                    onClick={() => run(Service.AddList({ name: c.name, source: "url", url: c.url, format: c.format, action: c.action } as any))}>
                    {t("rules.lists.addShort")}
                  </Chip>
                )}
              </div>
            );
          })}
        </div>
      )}
    </div>
  );
}

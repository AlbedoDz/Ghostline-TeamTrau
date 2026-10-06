import { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type AdvRow, type Settings } from "../../../../app/api";
import { useGhost } from "../../../../app/store";
import { describeError } from "../../../../i18n";
import css from "../../advanced.module.css";
import tc from "./tools.module.css";

const protocols = ["doh", "dot", "doq", "dnscrypt"];
const tags = ["no-log", "no-filter", "dnssec"];
const DEFAULT_MAX_SCAN = 500;

/** Scanner grades many DNS servers at once (spec 3 §6). */
export function Scanner() {
  const { t } = useTranslation();
  const settings = useGhost((s) => s.settings);
  const progress = useGhost((s) => s.advScan);
  const [mode, setMode] = useState<"catalog" | "paste">("catalog");
  const [protos, setProtos] = useState<string[]>([]);
  const [wantTags, setWantTags] = useState<string[]>([]);
  const [pinnedOnly, setPinnedOnly] = useState(false);
  const [pasted, setPasted] = useState("");
  const [bad, setBad] = useState<string[]>([]);
  const [rows, setRows] = useState<AdvRow[]>([]);
  const [selected, setSelected] = useState<string[]>([]);
  const [onlyDNSSEC, setOnlyDNSSEC] = useState(false);
  const [onlyAds, setOnlyAds] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [count, setCount] = useState<number | null>(null);

  const load = () => void Service.AdvancedResults().then((r) => setRows(r ?? []));
  useEffect(load, []);
  useEffect(() => {
    if (progress && !progress.running) load();
  }, [progress?.running]);

  const filter = { protocols: protos, tags: wantTags, sources: [] as string[], pinnedOnly };
  useEffect(() => {
    if (mode !== "catalog") return;
    void Service.CountScanServers(filter).then((n) => setCount(n ?? 0));
  }, [mode, protos, wantTags, pinnedOnly]);

  const maxScan = settings?.tools?.scanner?.maxServers || DEFAULT_MAX_SCAN;
  const running = !!progress?.running;
  const toggle = (list: string[], v: string) => (list.includes(v) ? list.filter((x) => x !== v) : [...list, v]);

  const start = async () => {
    setError(null);
    setBad([]);
    const poisonDomains = settings?.probeSites ?? [];
    const req = mode === "catalog"
      ? { filter, pasted: "", poisonDomains }
      : { pasted, poisonDomains };
    try {
      const st = await Service.StartAdvancedScan(req as any);
      setBad(st?.bad ?? []);
      setNote(st?.skipped ? t("tools.scanner.capped", { max: st.total, skipped: st.skipped }) : null);
      useGhost.getState().setAdvScan({ done: 0, total: st?.total ?? 0, running: true } as any);
    } catch (e) {
      setError(describeError(e));
    }
  };

  const run = (p: Promise<unknown>, ok?: string) =>
    p.then(() => { setError(null); if (ok) setNote(ok); load(); }).catch((e) => setError(describeError(e)));

  const shown = useMemo(
    () => rows.filter((r) => (!onlyDNSSEC || r.result.dnssec === "yes") && (!onlyAds || r.result.adFilter === "yes")),
    [rows, onlyDNSSEC, onlyAds],
  );

  return (
    <>
      <div className={css.panel}>
        <div className={css.row}>
          <label><input type="radio" name="advMode" checked={mode === "catalog"} onChange={() => setMode("catalog")} /> {t("tools.scanner.fromList")}</label>
          <label><input type="radio" name="advMode" checked={mode === "paste"} onChange={() => setMode("paste")} /> {t("tools.scanner.fromPaste")}</label>
        </div>
        {mode === "catalog" ? (
          <div className={tc.sources}>
            {protocols.map((p) => (
              <label key={p}><input type="checkbox" checked={protos.includes(p)} onChange={() => setProtos(toggle(protos, p))} /> {p}</label>
            ))}
            {tags.map((g) => (
              <label key={g}><input type="checkbox" checked={wantTags.includes(g)} onChange={() => setWantTags(toggle(wantTags, g))} /> {g}</label>
            ))}
            <label><input type="checkbox" checked={pinnedOnly} onChange={() => setPinnedOnly(!pinnedOnly)} /> {t("tools.scanner.pinnedOnly")}</label>
            {count !== null && (
              <span className={count > maxScan ? css.warn : css.dim}>
                {t("tools.scanner.matching", { count })}{count > maxScan ? ` · ${t("tools.scanner.willCap", { max: maxScan })}` : ""}
              </span>
            )}
          </div>
        ) : (
          <textarea aria-label={t("tools.scanner.pasteLabel")} rows={4} className={tc.wide} value={pasted}
            placeholder="https://dns.example/dns-query" onChange={(e) => setPasted(e.target.value)} />
        )}
        {settings && <ScannerOptions settings={settings} onError={setError} />}
        <div className={css.row}>
          <button onClick={() => void start()} disabled={running}>{t("tools.scan")}</button>
          {running && <button onClick={() => void Service.CancelAdvancedScan()}>{t("tools.cancel")}</button>}
          {progress && progress.total > 0 && <span className={css.dim}>{progress.done}/{progress.total}</span>}
        </div>
        {running && progress && progress.total > 0 && (
          <div className={tc.bar}><div className={tc.barFill} style={{ width: `${(100 * progress.done) / progress.total}%` }} /></div>
        )}
        {bad.length > 0 && <div className={css.warn}>{t("tools.scanner.badLines", { lines: bad.join(", ") })}</div>}
        {error && <div className={css.bad}>✕ {error}</div>}
        {note && <div className={css.ok}>{note}</div>}
      </div>

      {rows.length > 0 && (
        <div className={css.panel}>
          <div className={css.row}>
            <label><input type="checkbox" aria-label={t("tools.scanner.onlyDNSSEC")} checked={onlyDNSSEC} onChange={() => setOnlyDNSSEC(!onlyDNSSEC)} /> {t("tools.scanner.onlyDNSSEC")}</label>
            <label><input type="checkbox" aria-label={t("tools.scanner.onlyAds")} checked={onlyAds} onChange={() => setOnlyAds(!onlyAds)} /> {t("tools.scanner.onlyAds")}</label>
            <button disabled={selected.length === 0} onClick={() => void run(Service.SetPinnedMany(selected, true))}>{t("tools.scanner.pinSelected")}</button>
            <button onClick={() => void run(Service.ExportAdvancedCSV())}>{t("tools.scanner.exportCsv")}</button>
          </div>
          <table className={tc.table}>
            <thead>
              <tr>
                <th />
                <th>{t("tools.scanner.col.server")}</th>
                <th>{t("tools.scanner.col.median")}</th>
                <th>p90</th>
                <th>jitter</th>
                <th>{t("tools.scanner.col.loss")}</th>
                <th>DNSSEC</th>
                <th>{t("tools.scanner.col.ads")}</th>
                <th>{t("tools.scanner.col.poison")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {shown.map((r) => {
                const id = r.server.id;
                const res = r.result;
                const down = !res.reach?.ok;
                return (
                  <tr key={id}>
                    <td><input type="checkbox" aria-label={t("tools.select", { what: r.server.name })} checked={selected.includes(id)} onChange={() => setSelected(toggle(selected, id))} /></td>
                    <td>{r.server.name}<span className={css.dim}> {r.server.protocol}</span></td>
                    {down ? (
                      <td colSpan={7} className={css.bad}>{t("tools.scanner.unreachable", { reason: res.reach?.reason ?? "" })}</td>
                    ) : (
                      <>
                        <td>{res.medianMs} ms</td>
                        <td>{res.p90Ms} ms</td>
                        <td>{res.jitterMs.toFixed(1)}</td>
                        <td>{Math.round(res.loss * 100)}%</td>
                        <td>{t(`tools.tri.${res.dnssec}`)}</td>
                        <td>{t(`tools.tri.${res.adFilter}`)}</td>
                        <td className={(res.poisoned ?? []).length ? css.bad : undefined}>{(res.poisoned ?? []).join(", ") || "—"}</td>
                      </>
                    )}
                    <td>
                      {r.pasted ? (
                        <button onClick={() => void run(Service.AddScannedServers([id]), t("tools.scanner.added"))}>{t("tools.scanner.add")}</button>
                      ) : (
                        !down && <button onClick={() => void run(Service.UseOnlyServer(id))}>{t("tools.scanner.useOnly")}</button>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </>
  );
}

function ScannerOptions({ settings, onError }: { settings: Settings; onError: (e: string | null) => void }) {
  const { t } = useTranslation();
  const sc = settings.tools?.scanner ?? { rounds: 5, workers: 8, timeoutMs: 3000, maxServers: DEFAULT_MAX_SCAN };
  const [v, setV] = useState({
    rounds: String(sc.rounds), workers: String(sc.workers), timeoutMs: String(sc.timeoutMs),
    maxServers: String(sc.maxServers || DEFAULT_MAX_SCAN),
  });
  const save = () => {
    const scanner = { rounds: Number(v.rounds), workers: Number(v.workers), timeoutMs: Number(v.timeoutMs), maxServers: Number(v.maxServers) };
    const next = { ...settings, tools: { ...settings.tools, scanner } } as Settings;
    void Service.SaveSettings(next).then(() => { onError(null); useGhost.getState().setSettings(next); }).catch((e) => onError(describeError(e)));
  };
  const field = (k: keyof typeof v, label: string) => (
    <div className={css.setting}>
      <span>{label}</span>
      <input type="number" aria-label={label} style={{ width: 90 }} value={v[k]} onChange={(e) => setV({ ...v, [k]: e.target.value })} onBlur={save} />
    </div>
  );
  return (
    <details>
      <summary>{t("tools.options")}</summary>
      {field("rounds", t("tools.scanner.rounds"))}
      {field("workers", t("tools.scanner.workers"))}
      {field("timeoutMs", t("tools.scanner.timeoutMs"))}
      {field("maxServers", t("tools.scanner.maxServers"))}
      <div className={css.dim}>{t("tools.scanner.maxServersNote")}</div>
    </details>
  );
}

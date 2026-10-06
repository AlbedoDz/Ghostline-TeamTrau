import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type CFView, type CfResult, type Settings } from "../../../../app/api";
import { useGhost } from "../../../../app/store";
import { describeError, tCode } from "../../../../i18n";
import css from "../../full.module.css";
import tc from "./tools.module.css";

const DEFAULT_HOST = "speed.cloudflare.com";
const MAX_RULE_IPS = 4;

/** CfScan finds Cloudflare IPs that work on this network (spec 3 §7). */
export function CfScan() {
  const { t } = useTranslation();
  const settings = useGhost((s) => s.settings);
  const progress = useGhost((s) => s.cfScan);
  const [view, setView] = useState<CFView | null>(null);
  const [selected, setSelected] = useState<string[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [ruleOpen, setRuleOpen] = useState(false);
  const [rechecking, setRechecking] = useState(false);

  const load = () => void Service.GetCFView().then(setView);
  useEffect(load, []);
  useEffect(() => {
    if (!progress || progress.running) return;
    load();
    setError(progress.error ? tCode(`errors.${progress.error}.message`) : null);
    setNote(progress.note ? t(`tools.cfscan.note.${progress.note}`) : null);
  }, [progress?.running, progress?.error, progress?.note]);

  const running = !!progress?.running;
  const toggle = (ip: string) => setSelected((s) => (s.includes(ip) ? s.filter((x) => x !== ip) : [...s, ip]));

  const start = () => {
    setError(null);
    setNote(null);
    void Service.StartCFScan()
      .then(() => useGhost.getState().setCfScan({ phase: "probe", tried: 0, ok: 0, total: settings?.tools?.cfscan?.maxIps ?? 0, running: true } as any))
      .catch((e) => setError(describeError(e)));
  };

  const recheck = () => {
    setRechecking(true);
    void Service.RecheckCF(selected)
      .then(load)
      .catch((e) => setError(describeError(e)))
      .finally(() => setRechecking(false));
  };

  const copy = () => void navigator.clipboard?.writeText(selected.join("\n"));

  const results: CfResult[] = view?.results ?? [];
  return (
    <>
      <div className={css.panel}>
        {settings && <CfOptions settings={settings} onError={setError} />}
        <div className={css.row}>
          <button onClick={start} disabled={running}>{t("tools.scan")}</button>
          {running && <button onClick={() => void Service.CancelCFScan()}>{t("tools.cancel")}</button>}
          {progress && running && (
            <span className={css.dim}>
              {progress.phase === "speed"
                ? t("tools.cfscan.speedPhase")
                : t("tools.cfscan.probePhase", { tried: `${progress.tried}/${progress.total}`, ok: progress.ok })}
            </span>
          )}
        </div>
        {running && progress && progress.total > 0 && (
          <div className={tc.bar}><div className={tc.barFill} style={{ width: `${Math.min(100, (100 * progress.tried) / progress.total)}%` }} /></div>
        )}
        {error && <div className={css.bad}>✕ {error}</div>}
        {note && <div className={css.warn}>{note}</div>}
        <div className={css.dim}>{t("tools.cfscan.direct")}</div>
      </div>

      {results.length > 0 && (
        <div className={css.panel}>
          <div className={css.row}>
            {view?.scannedAt && !view.running && (
              <span className={css.dim}>{t("tools.cfscan.scannedAt", { time: new Date(view.scannedAt).toLocaleString() })}</span>
            )}
            <button disabled={selected.length === 0} onClick={copy}>{t("tools.copy")}</button>
            <button disabled={selected.length === 0} onClick={() => setRuleOpen(true)}>{t("tools.cfscan.createRule")}</button>
            <button disabled={selected.length === 0 || running || rechecking} onClick={recheck}>
              {rechecking ? t("tools.cfscan.rechecking") : t("tools.cfscan.recheck")}
            </button>
          </div>
          <table className={tc.table}>
            <thead>
              <tr>
                <th />
                <th>IP</th>
                <th>{t("tools.cfscan.col.latency")}</th>
                <th>PoP</th>
                <th>Mbit/s</th>
              </tr>
            </thead>
            <tbody>
              {results.map((r) => (
                <tr key={r.ip}>
                  <td><input type="checkbox" aria-label={t("tools.select", { what: r.ip })} checked={selected.includes(r.ip)} onChange={() => toggle(r.ip)} /></td>
                  <td>{r.ip}</td>
                  <td className={r.ok ? undefined : css.bad}>{r.ok ? `${r.latencyMs} ms` : r.reason}</td>
                  <td>{r.colo}</td>
                  <td>{r.mbps ? r.mbps.toFixed(1) : "—"}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}

      {ruleOpen && (
        <RuleDialog
          ips={selected}
          onToggle={toggle}
          onClose={(created) => {
            setRuleOpen(false);
            if (created) setNote(t("tools.cfscan.ruleCreated"));
          }}
        />
      )}
    </>
  );
}

function RuleDialog({ ips, onToggle, onClose }: { ips: string[]; onToggle: (ip: string) => void; onClose: (created: boolean) => void }) {
  const { t } = useTranslation();
  const [suggest, setSuggest] = useState<string[]>([]);
  const [text, setText] = useState("");
  const [errors, setErrors] = useState<string[]>([]);
  useEffect(() => void Service.CFSuggestDomains().then((d) => setSuggest(d ?? [])), []);

  const add = (p: string) => setText((cur) => (cur.split("\n").includes(p) ? cur : cur ? `${cur}\n${p}` : p));
  const patterns = text.split("\n").map((l) => l.trim()).filter(Boolean);
  const submit = async () => {
    try {
      const errs = (await Service.CreateCFRules(patterns, ips)) ?? [];
      if (errs.length) setErrors(errs.map((e) => (e.line ? `${patterns[e.line - 1] ?? ""}: ${e.msg}` : e.msg)));
      else onClose(true);
    } catch (e) {
      setErrors([describeError(e)]);
    }
  };
  return (
    <div className={css.dialog} role="dialog">
      <div className={css.dialogBox}>
        <div className={css.panelTitle}>{t("tools.cfscan.createRule")}</div>
        <div className={css.dim}>{t("tools.cfscan.ruleNote")}</div>
        <div className={tc.sources}>
          {ips.map((ip) => (
            <label key={ip}><input type="checkbox" aria-label={t("tools.select", { what: ip })} checked onChange={() => onToggle(ip)} /> {ip}</label>
          ))}
        </div>
        {ips.length > MAX_RULE_IPS && <div className={css.warn}>{t("tools.cfscan.tooManyIps", { max: MAX_RULE_IPS })}</div>}
        <textarea aria-label={t("tools.cfscan.domains")} rows={4} value={text} placeholder="example.com" onChange={(e) => setText(e.target.value)} />
        {suggest.length > 0 && (
          <div className={tc.sources}>
            <span className={css.dim}>{t("tools.cfscan.suggest")}</span>
            {suggest.map((s) => <button key={s} onClick={() => add(s)}>{s}</button>)}
          </div>
        )}
        {errors.map((e, i) => <div key={i} className={css.bad}>✕ {e}</div>)}
        <div className={css.row}>
          <button onClick={() => void submit()} disabled={ips.length === 0 || ips.length > MAX_RULE_IPS || patterns.length === 0}>{t("tools.cfscan.create")}</button>
          <button className={css.dim} onClick={() => onClose(false)}>{t("common.cancel")}</button>
        </div>
      </div>
    </div>
  );
}

function CfOptions({ settings, onError }: { settings: Settings; onError: (e: string | null) => void }) {
  const { t } = useTranslation();
  const cf = settings.tools?.cfscan;
  const [v, setV] = useState({
    host: cf?.host ?? DEFAULT_HOST, maxIps: String(cf?.maxIps ?? 2000), want: String(cf?.want ?? 50),
    concurrency: String(cf?.concurrency ?? 64), timeoutMs: String(cf?.timeoutMs ?? 2000),
  });
  const save = (patch: Partial<NonNullable<typeof cf>> = {}, nextV = v) => {
    const cfscan = {
      ...cf!, host: nextV.host.trim(), maxIps: Number(nextV.maxIps), want: Number(nextV.want),
      concurrency: Number(nextV.concurrency), timeoutMs: Number(nextV.timeoutMs), ...patch,
    };
    const next = { ...settings, tools: { ...settings.tools, cfscan } } as Settings;
    void Service.SaveSettings(next).then(() => { onError(null); useGhost.getState().setSettings(next); }).catch((e) => onError(describeError(e)));
  };
  const field = (k: keyof typeof v, label: string, type = "number") => (
    <div className={css.setting}>
      <span>{label}</span>
      <input type={type} aria-label={label} style={{ width: type === "text" ? 200 : 90 }} value={v[k]}
        onChange={(e) => setV({ ...v, [k]: e.target.value })} onBlur={() => save()} />
    </div>
  );
  return (
    <details>
      <summary>{t("tools.options")}</summary>
      {field("host", t("tools.cfscan.host"), "text")}
      <button onClick={() => { const nv = { ...v, host: DEFAULT_HOST }; setV(nv); save({}, nv); }}>{t("tools.cfscan.restoreDefault")}</button>
      {field("maxIps", t("tools.cfscan.maxIps"))}
      {field("want", t("tools.cfscan.want"))}
      {field("concurrency", t("tools.cfscan.concurrency"))}
      {field("timeoutMs", t("tools.cfscan.timeoutMs"))}
      <div className={css.setting}>
        <span>{t("tools.cfscan.speedTest")}</span>
        <input type="checkbox" aria-label={t("tools.cfscan.speedTest")} checked={!!cf?.speedTest} onChange={(e) => save({ speedTest: e.target.checked })} />
      </div>
    </details>
  );
}

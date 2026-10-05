import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type LookupResult, type LookupSource } from "../../../../app/api";
import { describeError, tCode } from "../../../../i18n";
import css from "../../advanced.module.css";
import tc from "./tools.module.css";

type Option = { src: LookupSource; checked: boolean; plain: boolean };

const key = (s: LookupSource) => `${s.kind}:${s.ref}`;

/** Lookup queries one name through several sources and compares (spec 3 §5). */
export function Lookup() {
  const { t } = useTranslation();
  const [types, setTypes] = useState<string[]>(["A"]);
  const [qtype, setQtype] = useState("A");
  const [name, setName] = useState("");
  const [options, setOptions] = useState<Option[]>([]);
  const [address, setAddress] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [errorCode, setErrorCode] = useState<string | null>(null);
  const [result, setResult] = useState<LookupResult | null>(null);
  const [details, setDetails] = useState(false);

  useEffect(() => {
    void Service.LookupTypes().then((ts) => ts?.length && setTypes(ts));
    void Promise.all([Service.DefaultLookupSources(), Service.ISPResolvers()]).then(([def, isp]) => {
      const opts: Option[] = (def ?? []).map((src) => ({ src, checked: true, plain: false }));
      // The unencrypted ISP source is never pre-selected (spec 3 §3).
      for (const ip of isp ?? []) opts.push({ src: { kind: "isp", ref: ip, label: ip }, checked: false, plain: true });
      setOptions(opts);
    });
  }, []);

  const toggle = (k: string) => setOptions((os) => os.map((o) => (key(o.src) === k ? { ...o, checked: !o.checked } : o)));

  const addAddress = () => {
    const a = address.trim();
    if (!a) return;
    const isIP = /^[0-9.]+$|^[0-9a-f:]+$/i.test(a) && !a.includes("://");
    const src: LookupSource = isIP ? { kind: "isp", ref: a, label: a } : { kind: "address", ref: a, label: a };
    setOptions((os) => (os.some((o) => key(o.src) === key(src)) ? os : [...os, { src, checked: true, plain: isIP }]));
    setAddress("");
  };

  const run = async () => {
    setBusy(true);
    setError(null);
    setErrorCode(null);
    try {
      const srcs = options.filter((o) => o.checked).map((o) => o.src);
      setResult(await Service.Lookup(name.trim(), qtype, srcs));
      setDetails(false);
    } catch (e) {
      setResult(null);
      setError(describeError(e));
      const m = /^([A-Z][A-Z0-9_]+)/.exec(e instanceof Error ? e.message : String(e));
      setErrorCode(m ? m[1] : null);
    } finally {
      setBusy(false);
    }
  };

  const overall = (result?.overall ?? "") as string;
  return (
    <>
      <div className={css.panel}>
        <div className={css.row}>
          <input aria-label={t("tools.lookup.name")} placeholder={t("tools.lookup.placeholder")} value={name}
            className={tc.grow} onChange={(e) => setName(e.target.value)} onKeyDown={(e) => e.key === "Enter" && void run()} />
          <select aria-label={t("tools.lookup.type")} value={qtype} onChange={(e) => setQtype(e.target.value)}>
            {types.map((ty) => <option key={ty} value={ty}>{ty}</option>)}
          </select>
          <button onClick={() => void run()} disabled={busy || !name.trim() || !options.some((o) => o.checked)}>{t("tools.lookup.run")}</button>
        </div>
        <div className={tc.sources}>
          {options.map((o) => (
            <label key={key(o.src)} className={o.plain ? tc.plain : undefined}>
              <input type="checkbox" checked={o.checked} onChange={() => toggle(key(o.src))} />
              {o.src.kind === "isp" ? t("tools.lookup.isp", { ip: o.src.ref }) : o.src.label}
              {o.plain && <span className={tc.warnTag}> ⚠ {t("tools.lookup.unencrypted")}</span>}
            </label>
          ))}
        </div>
        <div className={css.row}>
          <input aria-label={t("tools.lookup.addSource")} placeholder={t("tools.lookup.addSourceHint")} value={address}
            className={tc.grow} onChange={(e) => setAddress(e.target.value)} onKeyDown={(e) => e.key === "Enter" && addAddress()} />
          <button onClick={addAddress}>{t("tools.lookup.add")}</button>
        </div>
        <div className={css.dim}>{t("tools.lookup.ispNote")}</div>
      </div>

      {error && (
        <div className={css.panel}>
          <div className={css.bad}>✕ {error}</div>
          {errorCode && <div className={css.dim}>{tCode(`errors.${errorCode}.action`)}</div>}
        </div>
      )}

      {result && (
        <>
          {overall && (
            <div className={`${css.panel} ${tc.verdict} ${tc[overall] ?? ""}`}>
              <div className={tc.verdictTitle}>{t(`tools.lookup.verdict.${overall}.title`)}</div>
              <div className={css.dim}>{t(`tools.lookup.verdict.${overall}.body`)}</div>
            </div>
          )}
          <div className={css.panel}>
            <table className={tc.table}>
              <thead>
                <tr>
                  <th>{t("tools.lookup.col.source")}</th>
                  <th>{t("tools.lookup.col.status")}</th>
                  <th>{t("tools.lookup.col.latency")}</th>
                  <th>{t("tools.lookup.col.answer")}</th>
                  <th>AD</th>
                </tr>
              </thead>
              <tbody>
                {(result.answers ?? []).map((a, i) => {
                  const v = (result.verdicts?.[i] ?? "") as string;
                  return (
                    <tr key={i}>
                      <td>{a.source.label}</td>
                      <td className={v === "poisoned" || !a.ok ? css.bad : css.ok}>
                        {a.ok ? (v ? t(`tools.lookup.per.${v}`) : a.rcode) : t(`tools.lookup.fail.${a.error ?? "error"}`)}
                      </td>
                      <td>{a.ok ? `${a.latencyMs} ms` : "—"}</td>
                      <td>
                        {(a.records ?? []).length === 0 && a.ok ? a.rcode : null}
                        {(a.records ?? []).map((r, j) => <div key={j}>{r.data}</div>)}
                      </td>
                      <td>{a.ad ? "✓" : ""}</td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            <button onClick={() => setDetails((d) => !d)} aria-expanded={details}>{t("tools.lookup.details")}</button>
            {details && (result.answers ?? []).map((a, i) => a.dig && (
              <div key={i}>
                <pre className={css.code}>{a.dig}</pre>
                <button onClick={() => void navigator.clipboard?.writeText(a.dig)}>{t("tools.copy")}</button>
              </div>
            ))}
          </div>
        </>
      )}
    </>
  );
}

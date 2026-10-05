import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type CatalogItem, type FakeSNIView } from "../../../app/api";
import { useGhost } from "../../../app/store";
import { describeError, tCode } from "../../../i18n";
import { Toggle } from "../../../components/neon/Toggle";
import { Chip } from "../../../components/neon/Chip";
import { FakeSniWarning } from "./FakeSniWarning";
import { actionText } from "./rulesFormat";
import css from "../advanced.module.css";

const COUNTERS = [
  ["fakesni", "fakesni"],
  ["fallback", "fakesni_fallback"],
  ["clientRejected", "fakesni_client_rejected"],
  ["verifyFailed", "fakesni_verify_failed"],
] as const;

export function FakeSni() {
  const { t } = useTranslation();
  const settings = useGhost((s) => s.settings);
  const snap = useGhost((s) => s.snapshot);
  const stats = useGhost((s) => s.proxyStats);
  const rulesVersion = useGhost((s) => s.rulesVersion);
  const [view, setView] = useState<FakeSNIView | null>(null);
  const [presets, setPresets] = useState<CatalogItem[]>([]);
  const [error, setError] = useState<string | null>(null);

  const reload = () => void Service.GetFakeSNIView().then(setView);
  useEffect(reload, [rulesVersion, settings?.fakeSni?.enabled, settings?.fakeSni?.ackVersion]);
  useEffect(() => {
    void Service.Catalog().then((c) => setPresets((c ?? []).filter((i) => i.category === "fakesni")));
  }, []);

  if (!settings || !view) return null;

  const run = (p: Promise<unknown>) => p.then(() => { setError(null); reload(); }).catch((e) => setError(describeError(e)));

  if (!view.ack) {
    return (
      <div className={css.page}>
        <div className={css.head}><span>{t("fakesni.title")}</span></div>
        <FakeSniWarning onAck={() => void run(Service.AckFakeSNIWarning())} />
      </div>
    );
  }

  const fs = snap.fakeSni;
  const lists = view.lists ?? [];
  const presetList = (p: CatalogItem) => lists.find((l) => l.url === p.url);
  const togglePreset = (p: CatalogItem, on: boolean) => {
    const cur = presetList(p);
    if (on && !cur) {
      void run(Service.AddList({ name: p.name, source: "url", url: p.url, format: p.format, action: p.action,
        signed: p.signed, trustedForSNI: p.trustedForSNI } as any));
    } else if (!on && cur) {
      void run(Service.DeleteList(cur.id));
    }
  };
  const byOutcome = (stats ?? view.stats)?.byOutcome ?? {};

  return (
    <div className={css.page}>
      <div className={css.head}>
        <span>{t("fakesni.title")}</span>
        <span className={css.count}>{fs?.active ? t("overview.fakesniDomains", { count: fs.domains }) : t("common.off")}</span>
      </div>

      {fs?.error && fs.error.code !== "FAKESNI_NEEDS_PROXY" && (
        <div className={css.panel}>
          <div className={css.bad}>✕ {tCode(`errors.${fs.error.code}.message`, fs.error.params ?? undefined)}</div>
          <div className={css.row}><Chip onClick={() => void run(Service.RetryFakeSNI())}>{t("common.retry")}</Chip></div>
        </div>
      )}

      <div className={css.panel}>
        <div className={css.setting}>
          <span>{t("fakesni.enabled")}</span>
          <Toggle label={t("fakesni.enabled")} checked={view.enabled} onChange={(v) => void run(Service.SetFakeSNI(v))} />
        </div>
        {!settings.proxy?.enabled && (
          <div className={css.warn}>
            ⚠ {t("fakesni.needsProxy")}{" "}
            <button className={css.ok} onClick={() => void run(Service.SetProxyEnabled(true))}>{t("fakesni.enableProxy")}</button>
          </div>
        )}
        {error && <div className={css.bad}>{error}</div>}
        <div className={css.dim}>{t("fakesni.firefox")}</div>
      </div>

      <div className={css.grid2}>
        <div className={css.panel}>
          <div className={css.panelTitle}>{t("fakesni.presets")}</div>
          {presets.map((p) => {
            const cur = presetList(p);
            return (
              <div key={p.id} className={css.setting}>
                <span>
                  {p.name}
                  {cur?.lastError && <span className={css.bad}> · {tCode(`errors.${cur.lastError}.message`, { id: p.name })}</span>}
                </span>
                <Toggle label={p.name} checked={!!cur} onChange={(v) => togglePreset(p, v)} />
              </div>
            );
          })}
        </div>
        <div className={css.panel}>
          <div className={css.panelTitle}>{t("fakesni.ca")}</div>
          {fs?.active ? (
            <>
              <div className={css.ok}>{t("fakesni.caInfo", { domains: fs.domains, date: String(fs.notAfter).slice(0, 10) })}</div>
              <code className={css.dim} style={{ wordBreak: "break-all" }}>{fs.thumbprint}</code>
            </>
          ) : (
            <div className={css.dim}>{t("fakesni.caNone")}</div>
          )}
          <div className={css.row} style={{ flexWrap: "wrap", gap: 12, marginTop: 8 }}>
            {COUNTERS.map(([key, outcome]) => (
              <span key={key} className={css.dim}>{t(`fakesni.stats.${key}`, { n: byOutcome[outcome] ?? 0 })}</span>
            ))}
          </div>
        </div>
      </div>

      <div className={css.panel}>
        <div className={css.panelTitle}>{t("fakesni.rules")}</div>
        {(view.rules ?? []).length === 0 && <div className={css.dim}>{t("fakesni.noRules")}</div>}
        {(view.rules ?? []).map((r, i) => (
          <div key={i + r.pattern} className={css.setting}>
            <span>{r.pattern}</span>
            <span className={css.dim}>{actionText(r as any, t)}</span>
          </div>
        ))}
        <Chip onClick={() => useGhost.getState().setPage("rules")}>{t("nav.rules")}</Chip>
      </div>
    </div>
  );
}

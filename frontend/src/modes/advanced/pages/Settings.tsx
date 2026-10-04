import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Browser } from "@wailsio/runtime";
import { Service, type Adapter } from "../../../app/api";
import { useGhost } from "../../../app/store";
import { useUpdate } from "../../../app/format";
import { saveSettings } from "../../../app/settings";
import { describeError, initI18n } from "../../../i18n";
import { Toggle } from "../../../components/neon/Toggle";
import { Chip } from "../../../components/neon/Chip";
import css from "../advanced.module.css";

export function Settings() {
  const { t } = useTranslation();
  const settings = useGhost((s) => s.settings);
  const snap = useGhost((s) => s.snapshot);
  const info = useGhost((s) => s.info);
  const [error, setError] = useState<string | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [adapters, setAdapters] = useState<Adapter[]>([]);
  const [bootstrap, setBootstrap] = useState((settings?.bootstrap ?? []).join("\n"));
  const [testDomain, setTestDomain] = useState(settings?.testDomain ?? "");
  const [confirmService, setConfirmService] = useState<string | null>(null);
  const [check, setCheck] = useState<{ busy?: boolean; text?: string; error?: boolean } | null>(null);

  const checkUpdate = () => {
    setCheck({ busy: true });
    Service.CheckUpdateNow()
      .then((r) => {
        if (r.newer) {
          useGhost.getState().setUpdate({ tag: r.latest, url: r.url });
          setCheck(null);
        } else setCheck({ text: t("settings.upToDate", { version: r.current }) });
      })
      .catch((e) => setCheck({ text: describeError(e), error: true }));
  };

  useEffect(() => {
    void Service.ListAdapters().then((a) => setAdapters(a ?? []));
  }, []);

  if (!settings) return null;
  const save = async (patch: Parameters<typeof saveSettings>[0]) => {
    const err = await saveSettings(patch);
    setError(err);
    setNote(err ? null : t("settings.saved"));
  };

  const busyService = snap.error?.code === "PORT53_BUSY" ? String(snap.error.params?.service ?? "") : "";
  const manual = settings.adapters === "manual";
  const guids = settings.adapterGuids ?? [];
  const upd = useUpdate();
  const tag = upd?.tag;
  const url = upd?.url;

  const toggleRow = (label: string, checked: boolean, patch: (v: boolean) => Parameters<typeof saveSettings>[0]) => (
    <div className={css.setting}>
      <span>{label}</span>
      <Toggle label={label} checked={checked} onChange={(v) => void save(patch(v))} />
    </div>
  );

  return (
    <div className={css.page}>
      <div className={css.head}>
        <span>{t("settings.title")}</span>
        <span className={css.count}>
          {t("settings.version", { version: info?.version ?? "" })}
          {tag && url ? (
            <button className={css.ok} style={{ marginLeft: 8 }} onClick={() => void Browser.OpenURL(url)}>
              {t("settings.update", { tag })}
            </button>
          ) : (
            check && <span className={check.error ? css.bad : css.dim} style={{ marginLeft: 8 }}>{check.text}</span>
          )}
          <button className={css.ok} style={{ marginLeft: 8 }} disabled={check?.busy} onClick={checkUpdate}>
            {check?.busy ? t("settings.checking") : t("settings.checkUpdate")}
          </button>
        </span>
      </div>
      <div className={css.panel}>
        <div className={css.setting}>
          <span>{t("settings.language")}</span>
          <span className={css.row}>
            {(["vi", "en"] as const).map((l) => (
              <Chip key={l} active={settings.language === l} onClick={() => void initI18n(l).then(() => save((s) => ({ ...s, language: l })))}>
                {l.toUpperCase()}
              </Chip>
            ))}
          </span>
        </div>
        {toggleRow(t("settings.startWithWindows"), settings.startWithWindows, (v) => (s) => ({ ...s, startWithWindows: v }))}
        {toggleRow(t("settings.autoConnect"), settings.autoConnect, (v) => (s) => ({ ...s, autoConnect: v }))}
        {toggleRow(t("settings.closeToTray"), settings.closeToTray, (v) => (s) => ({ ...s, closeToTray: v }))}
        <div className={css.setting}>
          <span>{t("settings.adapters")}</span>
          <span className={css.row}>
            <Chip active={!manual} onClick={() => void save((s) => ({ ...s, adapters: "auto" }))}>{t("common.auto")}</Chip>
            <Chip active={manual} onClick={() => void save((s) => ({ ...s, adapters: "manual" }))}>{t("settings.adaptersManual")}</Chip>
          </span>
        </div>
        {manual &&
          adapters.map((a) => (
            <div key={a.guid} className={css.setting}>
              <span>{a.alias}</span>
              <Toggle
                label={a.alias}
                checked={guids.includes(a.guid)}
                onChange={(v) =>
                  void save((s) => ({ ...s, adapterGuids: v ? [...guids, a.guid] : guids.filter((g) => g !== a.guid) }))
                }
              />
            </div>
          ))}
        <div className={css.setting}>
          <span>{t("settings.testDomain")}</span>
          <input value={testDomain} onChange={(e) => setTestDomain(e.target.value)} onBlur={() => void save((s) => ({ ...s, testDomain }))} />
        </div>
        <div className={css.setting}>
          <span>{t("settings.bootstrap")}</span>
          <textarea
            style={{ width: 200, minHeight: 50 }}
            value={bootstrap}
            onChange={(e) => setBootstrap(e.target.value)}
            onBlur={() => void save((s) => ({ ...s, bootstrap: bootstrap.split(/\s+/).filter(Boolean) }))}
          />
        </div>
        <div className={css.setting}>
          <span>{t("settings.maxUpstreams")}</span>
          <input type="number" min={1} max={10} style={{ width: 70 }} value={settings.maxUpstreams}
            onChange={(e) => void save((s) => ({ ...s, maxUpstreams: Number(e.target.value) }))} />
        </div>
        {toggleRow(t("settings.updateServerList"), settings.updates.updateServerList, (v) => (s) => ({ ...s, updates: { ...s.updates, updateServerList: v } }))}
        {toggleRow(t("settings.checkApp"), settings.updates.checkApp, (v) => (s) => ({ ...s, updates: { ...s.updates, checkApp: v } }))}
        {error && <div className={css.bad}>{error}</div>}
        {note && !error && <div className={css.ok}>{note}</div>}
      </div>

      <div className={css.row}>
        <button
          className={css.danger}
          onClick={() =>
            void Service.RestoreDNSNow()
              .then(() => (setError(null), setNote(t("settings.restored"))))
              .catch((e) => setError(describeError(e)))
          }
        >
          {t("settings.restoreNow")}
        </button>
        {busyService && (
          <button className={css.danger} style={{ borderColor: "var(--amber)", color: "var(--amber)" }} onClick={() => setConfirmService(busyService)}>
            {t("settings.stopService", { name: busyService })}
          </button>
        )}
      </div>

      {confirmService && (
        <div className={css.dialog} role="dialog">
          <div className={css.dialogBox}>
            <div>{t("settings.stopServiceConfirm", { name: confirmService })}</div>
            <div className={css.row}>
              <button
                className={css.danger}
                onClick={() => {
                  void Service.StopConflictingService(confirmService).catch((e) => setError(describeError(e)));
                  setConfirmService(null);
                }}
              >
                {t("common.confirm")}
              </button>
              <button className={css.dim} onClick={() => setConfirmService(null)}>
                {t("common.cancel")}
              </button>
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

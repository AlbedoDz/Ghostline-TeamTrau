import { useCallback, useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service, type LineError, type Rule, type RulesView } from "../../../app/api";
import { useGhost } from "../../../app/store";
import { saveSettings } from "../../../app/settings";
import { describeError } from "../../../i18n";
import { Chip } from "../../../components/neon/Chip";
import { RulesTable } from "./RulesTable";
import { RulesText } from "./RulesText";
import { Lists } from "./Lists";
import { explainText, formatRules } from "./rulesFormat";
import css from "../full.module.css";

export function Rules() {
  const { t } = useTranslation();
  const settings = useGhost((s) => s.settings);
  const version = useGhost((s) => s.rulesVersion);
  const [view, setView] = useState<RulesView | null>(null);
  const [tab, setTab] = useState<"table" | "text">("table");
  const [text, setText] = useState("");
  const [errors, setErrors] = useState<LineError[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [host, setHost] = useState("");
  const [explained, setExplained] = useState<string | null>(null);

  const load = useCallback(() => {
    void Service.GetRules().then((v) => v && setView(v));
  }, []);
  useEffect(load, [load, version]);

  if (!view) return null;
  const rules = view.rules ?? [];
  const lists = view.lists ?? [];
  const upstreams = (settings?.proxy?.upstreams ?? []).map((u) => u.id);

  const saveTable = (next: Rule[]) => {
    setView({ ...view, rules: next });
    Service.SaveRulesTable(next)
      .then((errs) => {
        if (errs && errs.length) {
          setError(errs.map((e) => t("rules.lineError", { line: e.line, msg: e.msg })).join(" · "));
          load();
        } else setError(null);
      })
      .catch((e) => setError(describeError(e)));
  };

  const showText = () => {
    setText(formatRules(rules));
    setErrors([]);
    setTab("text");
  };

  const saveText = () => {
    Service.SaveRulesText(text)
      .then((errs) => {
        setErrors(errs ?? []);
        if (!errs || errs.length === 0) load();
      })
      .catch((e) => setError(describeError(e)));
  };

  const explain = () => {
    if (!host.trim()) return;
    void Service.Explain(host.trim()).then((d) => setExplained(explainText(d, lists, t)));
  };

  return (
    <div className={css.page}>
      <div className={css.head}>
        <span>{t("rules.title")}</span>
        <span className={css.row} role="tablist">
          <button role="tab" aria-selected={tab === "table"} className={css.chipTab} onClick={() => setTab("table")}>{t("rules.tabTable")}</button>
          <button role="tab" aria-selected={tab === "text"} className={css.chipTab} onClick={showText}>{t("rules.tabText")}</button>
        </span>
      </div>

      <div className={css.panel}>
        {tab === "table" ? (
          <RulesTable rules={rules} upstreams={upstreams} onSave={saveTable} />
        ) : (
          <>
            <RulesText value={text} onChange={setText} errors={errors} />
            <div className={css.row}>
              <Chip onClick={saveText}>{t("common.save")}</Chip>
            </div>
          </>
        )}
        {error && <div className={css.bad}>{error}</div>}
      </div>

      <div className={css.panel}>
        <div className={css.panelTitle}>{t("rules.explain.title")}</div>
        <div className={css.row}>
          <input aria-label={t("rules.explain.input")} placeholder="www.example.com" value={host}
            onChange={(e) => setHost(e.target.value)} onKeyDown={(e) => e.key === "Enter" && explain()} style={{ flex: 1 }} />
          <Chip onClick={explain}>{t("rules.explain.go")}</Chip>
        </div>
        {explained && <div className={css.ok}>{explained}</div>}
      </div>

      <Lists lists={lists} upstreams={upstreams} onChanged={load} />

      {settings && (
        <div className={css.panel}>
          <div className={css.setting}>
            <span>{t("rules.blockMode")}</span>
            <span className={css.row}>
              {(["zero", "nxdomain"] as const).map((m) => (
                <Chip key={m} active={settings.dnsBlockMode === m} onClick={() => void saveSettings((s) => ({ ...s, dnsBlockMode: m })).then(setError)}>
                  {t(`rules.blockModes.${m}`)}
                </Chip>
              ))}
            </span>
          </div>
          <div className={css.dim}>{t("rules.blockModeNote")}</div>
        </div>
      )}
    </div>
  );
}

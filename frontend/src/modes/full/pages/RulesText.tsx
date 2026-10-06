import { useTranslation } from "react-i18next";
import type { LineError } from "../../../app/api";
import css from "../full.module.css";

type Props = { value: string; onChange: (v: string) => void; errors: LineError[] };

/** RulesText is a plain textarea with a line-number gutter marking errors. */
export function RulesText({ value, onChange, errors }: Props) {
  const { t } = useTranslation();
  const bad = new Set(errors.map((e) => e.line));
  const lines = Math.max(1, value.split("\n").length);
  return (
    <div>
      <div className={css.row} style={{ alignItems: "stretch", gap: 0 }}>
        <div className={css.dim} style={{ textAlign: "right", paddingRight: 6, userSelect: "none", fontFamily: "inherit", lineHeight: "1.5" }} aria-hidden>
          {Array.from({ length: lines }, (_, i) => (
            <div key={i} data-testid={`line-${i + 1}`} data-error={bad.has(i + 1) ? "true" : "false"} className={bad.has(i + 1) ? css.bad : undefined}>
              {i + 1}
            </div>
          ))}
        </div>
        <textarea
          aria-label={t("rules.textLabel")}
          spellCheck={false}
          wrap="off"
          style={{ flex: 1, minHeight: 220, lineHeight: "1.5", fontFamily: "inherit" }}
          value={value}
          onChange={(e) => onChange(e.target.value)}
        />
      </div>
      {errors.map((e) => (
        <div key={e.line + e.msg} className={css.bad}>
          {t("rules.lineError", { line: e.line, msg: e.msg })}
        </div>
      ))}
      <div className={css.dim}>{t("rules.syntax")}</div>
    </div>
  );
}

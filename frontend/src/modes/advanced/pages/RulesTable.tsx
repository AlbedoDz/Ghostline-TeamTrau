import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { Rule } from "../../../app/api";
import { Toggle } from "../../../components/neon/Toggle";
import { Chip } from "../../../components/neon/Chip";
import { actionText } from "./rulesFormat";
import css from "../advanced.module.css";

type Props = { rules: Rule[]; upstreams: string[]; onSave: (next: Rule[]) => void };

const ACTIONS = ["block", "allow", "fragment=on", "fragment=off", "ip", "upstream"] as const;

function buildRule(pattern: string, action: string, value: string): Rule {
  const r: any = { pattern: pattern.trim(), enabled: true };
  if (action === "block") r.block = true;
  else if (action === "allow") r.allow = true;
  else if (action.startsWith("fragment=")) r.fragment = action.slice("fragment=".length);
  else if (action === "ip") r.ips = value.split(/[\s,]+/).filter(Boolean);
  else if (action === "upstream") r.upstream = value.trim();
  return r as Rule;
}

/** RulesTable edits user rules row by row; order sets priority. */
export function RulesTable({ rules, upstreams, onSave }: Props) {
  const { t } = useTranslation();
  const [pattern, setPattern] = useState("");
  const [action, setAction] = useState<string>("block");
  const [value, setValue] = useState("");

  const move = (i: number, d: number) => {
    const j = i + d;
    if (j < 0 || j >= rules.length) return;
    const next = rules.slice();
    [next[i], next[j]] = [next[j], next[i]];
    onSave(next);
  };

  return (
    <div>
      <table style={{ width: "100%" }}>
        <tbody>
          {rules.map((r, i) => (
            <tr key={i + r.pattern}>
              <td>
                <Toggle label={t("rules.enableFor", { pattern: r.pattern })} checked={r.enabled}
                  onChange={(v) => onSave(rules.map((x, k) => (k === i ? { ...x, enabled: v } : x)))} />
              </td>
              <td>{r.pattern}</td>
              <td className={css.dim}>
                {actionText(r as any, t)}
              </td>
              <td className={css.dim}>{r.comment}</td>
              <td>
                <span className={css.row}>
                  <Chip label={t("rules.up", { pattern: r.pattern })} onClick={() => move(i, -1)}>▲</Chip>
                  <Chip label={t("rules.down", { pattern: r.pattern })} onClick={() => move(i, 1)}>▼</Chip>
                  <Chip label={t("rules.delete", { pattern: r.pattern })} onClick={() => onSave(rules.filter((_, k) => k !== i))}>✕</Chip>
                </span>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {rules.length === 0 && <div className={css.dim}>{t("rules.empty")}</div>}
      <div className={css.row} style={{ marginTop: 8, flexWrap: "wrap" }}>
        <input aria-label={t("rules.pattern")} placeholder="example.com · *.ads.com · ~ads · 10.0.0.0/8" value={pattern}
          onChange={(e) => setPattern(e.target.value)} style={{ flex: 1, minWidth: 200 }} />
        <select aria-label={t("rules.actionLabel")} value={action} onChange={(e) => setAction(e.target.value)}>
          {ACTIONS.map((a) => (
            <option key={a} value={a}>{t(`rules.actions.${a}`)}</option>
          ))}
        </select>
        {action === "ip" && <input aria-label="ip" placeholder="1.2.3.4" value={value} onChange={(e) => setValue(e.target.value)} style={{ width: 140 }} />}
        {action === "upstream" && (
          <select aria-label={t("rules.actions.upstream")} value={value} onChange={(e) => setValue(e.target.value)}>
            <option value="" />
            {upstreams.map((u) => (
              <option key={u} value={u}>{u}</option>
            ))}
          </select>
        )}
        <Chip
          onClick={() => {
            if (!pattern.trim()) return;
            onSave([...rules, buildRule(pattern, action, value)]);
            setPattern("");
            setValue("");
          }}
        >
          {t("rules.add")}
        </Chip>
      </div>
    </div>
  );
}

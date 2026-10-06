import type { Decision, List, Rule } from "../../../app/api";

type R = Rule & { block?: boolean; allow?: boolean; ips?: string[] | null; fragment?: string; upstream?: string; sni?: string; connect?: string; sniIgnored?: boolean };

/** formatRule mirrors rules.FormatText in Go for one rule. */
export function formatRule(r: R): string {
  let s = (r.enabled ? "" : "#! ") + r.pattern;
  if (r.block) s += " block";
  if (r.allow) s += " allow";
  for (const ip of r.ips ?? []) s += ` ip=${ip}`;
  if (r.fragment) s += ` fragment=${r.fragment}`;
  if (r.upstream) s += ` upstream=${r.upstream}`;
  if (r.sni) s += ` sni=${r.sni}`;
  if (r.connect) s += ` connect=${r.connect}`;
  if (r.comment) s += `  # ${r.comment}`;
  return s;
}

export const formatRules = (rs: Rule[]) => rs.map((r) => formatRule(r as R) + "\n").join("");

type T = (key: string, opts?: Record<string, unknown>) => string;

/** actionText describes what a rule or decision does. */
export function actionText(a: R | Decision, t: T): string {
  const x = a as R;
  const parts: string[] = [];
  if (x.block) parts.push(t("rules.action.block"));
  if (x.allow) parts.push(t("rules.action.allow"));
  if (x.ips?.length) parts.push(t("rules.action.ip", { ips: x.ips.join(", ") }));
  if (x.fragment) parts.push(t("rules.action.fragment", { mode: t(`rules.frag.${x.fragment}`) }));
  if (x.upstream) parts.push(t("rules.action.upstream", { id: x.upstream }));
  if (x.sni) parts.push(t("rules.action.sni", { sni: x.sni }));
  if (x.connect) parts.push(t("rules.action.connect", { host: x.connect }));
  return parts.join(" · ");
}

/** explainText renders an Explain() result. */
export function explainText(d: Decision, lists: List[], t: T): string {
  const src = d.source;
  if (!src || !src.kind) return t("rules.explain.none");
  const what = actionText(d, t) || t("rules.explain.direct");
  const where =
    src.kind === "rule"
      ? t("rules.explain.rule", { n: (src.index ?? 0) + 1 })
      : t("rules.explain.list", { name: lists.find((l) => l.id === src.listId)?.name ?? src.listId, line: src.line });
  const d2 = d as unknown as R;
  const notes: string[] = [];
  if (d2.sni) notes.push(t("rules.explain.fakesni", { sni: d2.sni, connect: d2.connect || "—" }));
  if (d2.sniIgnored) notes.push(t("rules.explain.sniIgnored"));
  return [`${what} — ${where}`, ...notes].join(" · ");
}

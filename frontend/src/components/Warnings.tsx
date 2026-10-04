import { useTranslation } from "react-i18next";
import { Service } from "../app/api";
import { useGhost } from "../app/store";
import { tCode } from "../i18n";
import { Banner } from "./neon/Banner";

/** Persistent warnings. RESTORE_FAILED can only be cleared by restoring. */
export function Warnings() {
  const { t } = useTranslation();
  const warnings = useGhost((s) => s.snapshot.warnings) ?? [];
  if (warnings.length === 0) return null;
  return (
    <div style={{ display: "grid", gap: 6, padding: "0 14px 8px" }}>
      {warnings.map((w, i) => (
        <Banner
          key={w.code + i}
          tone={w.code === "RESTORE_FAILED" ? "err" : "warn"}
          actions={
            w.code === "RESTORE_FAILED"
              ? [{ label: t("settings.restoreNow"), onClick: () => void Service.RestoreDNSNow(), primary: true }]
              : [{ label: t("common.understood"), onClick: () => void Service.DismissWarning(w.code) }]
          }
        >
          {tCode(`errors.${w.code}.message`, w.params ?? undefined)}
        </Banner>
      ))}
    </div>
  );
}

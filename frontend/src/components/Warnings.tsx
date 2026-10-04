import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Service } from "../app/api";
import { useGhost } from "../app/store";
import { describeError, tCode } from "../i18n";
import { Banner } from "./neon/Banner";

/** Persistent warnings. RESTORE_FAILED can only be cleared by restoring. */
export function Warnings() {
  const { t } = useTranslation();
  const warnings = useGhost((s) => s.snapshot.warnings) ?? [];
  const [failure, setFailure] = useState<string | null>(null);
  if (warnings.length === 0) return null;

  const restoreProxy = () => {
    setFailure(null);
    Service.RestoreSystemProxy().catch((e) => setFailure(describeError(e)));
  };

  const restore = () => {
    setFailure(null);
    Service.RestoreDNSNow().catch((e) => setFailure(describeError(e)));
  };

  return (
    <div style={{ display: "grid", gap: 6, padding: "0 14px 8px" }}>
      {warnings.map((w, i) => (
        <Banner
          key={w.code + i}
          tone={w.code === "RESTORE_FAILED" ? "err" : "warn"}
          actions={
            w.code === "RESTORE_FAILED"
              ? [{ label: t("settings.restoreNow"), onClick: restore, primary: true }]
              : w.code === "SYSPROXY_RESTORE_FAILED"
                ? [{ label: t("errors.SYSPROXY_RESTORE_FAILED.button"), onClick: restoreProxy, primary: true }]
              : w.code === "SYSPROXY_EXISTING"
                ? [
                    { label: t("errors.SYSPROXY_EXISTING.override"), onClick: () => void Service.AnswerSysProxyOverride(true), primary: true },
                    { label: t("errors.SYSPROXY_EXISTING.keep"), onClick: () => void Service.AnswerSysProxyOverride(false) },
                  ]
                : [{ label: t("common.understood"), onClick: () => void Service.DismissWarning(w.code) }]
          }
        >
          {tCode(`errors.${w.code}.message`, w.params ?? undefined)}
          {w.code === "SYSPROXY_RESTORE_FAILED" && <div>{tCode("errors.SYSPROXY_RESTORE_FAILED.action")}</div>}
          {(w.code === "RESTORE_FAILED" || w.code === "SYSPROXY_RESTORE_FAILED") && failure && <div>✕ {failure}</div>}
        </Banner>
      ))}
    </div>
  );
}

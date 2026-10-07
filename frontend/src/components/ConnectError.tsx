import { useTranslation } from "react-i18next";
import { Service } from "../app/api";
import { useGhost } from "../app/store";
import { tCode } from "../i18n";
import { Banner, type BannerAction } from "./neon/Banner";

type Props = { onOpenServers: () => void; onOpenLogs: () => void };

/** ConnectError shows the last connect error with the actions that fix it (both modes). */
export function ConnectError({ onOpenServers }: Props) {
  const { t } = useTranslation();
  const snap = useGhost((s) => s.snapshot);
  const settings = useGhost((s) => s.settings);
  if (String(snap.status) !== "error" || !snap.error) return null;
  const code = snap.error.code;

  const saveAndConnect = async (patch: (s: NonNullable<typeof settings>) => NonNullable<typeof settings>) => {
    if (!settings) return;
    const next = patch(structuredClone(settings));
    await Service.SaveSettings(next);
    useGhost.getState().setSettings(next);
    void Service.Connect();
  };

  const actions: BannerAction[] = [{ label: t("common.retry"), onClick: () => void Service.Connect() }];
  if (code === "NO_SERVERS" && !settings?.fragmentDns?.enabled) {
    actions.push({ label: t("simple.enableFragment"), onClick: () => void saveAndConnect((s) => ({ ...s, fragmentDns: { ...s.fragmentDns, enabled: true } })) });
  }
  if (code === "NO_PINNED_SERVERS") {
    actions.unshift({ label: t("simple.openServers"), onClick: onOpenServers, primary: true });
    actions.push({ label: t("simple.disablePinnedOnly"), onClick: () => void saveAndConnect((s) => ({ ...s, pinnedOnly: false })) });
  }

  return (
    <Banner tone="err" actions={actions}>
      {tCode(`errors.${code}.message`, snap.error.params ?? undefined)}
      {code === "VERIFY_LEAK" && snap.error.params?.adapters ? (
        <div>{tCode("errors.VERIFY_LEAK.adapters", snap.error.params)}</div>
      ) : null}
    </Banner>
  );
}

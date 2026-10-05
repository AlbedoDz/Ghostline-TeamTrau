import type { TFunction } from "i18next";
import { useGhost } from "./store";

// LAN devices that used this PC's DNS in the last 10 minutes lose the
// internet when Ghostline disconnects, so ask first when there are any.
export function confirmDisconnect(t: TFunction): boolean {
  const st = useGhost.getState();
  const n = st.snapshot.dnsServer?.running ? st.dnsStats?.clients10m ?? 0 : 0;
  return n === 0 || window.confirm(t("dnsserver.disconnectConfirm", { count: n }));
}

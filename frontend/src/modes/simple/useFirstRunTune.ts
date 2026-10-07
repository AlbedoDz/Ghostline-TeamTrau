import { useEffect, useRef, useState } from "react";
import { Service, type Settings } from "../../app/api";
import { useGhost } from "../../app/store";

/** What the first-run tune is doing: waiting for the site check, or tuning. */
export type FirstRunPhase = null | "probe" | "tune";

/**
 * useFirstRunTune runs once, on the first connect after installing. Go
 * checks the test sites after DPI bypass (if on) is up and sets `probed`;
 * when some are still blocked at TLS, this auto-tunes DPI bypass on them.
 * Disconnecting mid-way leaves it for the next connect. The Simple
 * interface shows it as one more connect step.
 */
export function useFirstRunTune(status: string): FirstRunPhase {
  const checked = useGhost((s) => s.settings?.simple?.checked);
  const probed = useGhost((s) => !!s.snapshot.probed);
  const blocked = useGhost((s) => s.snapshot.blockedSites?.length ?? 0);
  const autotune = useGhost((s) => s.autotune);
  const [phase, setPhase] = useState<FirstRunPhase>(null);
  const sawTune = useRef(false);
  const connected = status === "protected" || status === "degraded";

  const finish = () => {
    sawTune.current = false;
    setPhase(null);
    void Service.MarkNetworkChecked().catch(() => {});
    const s = useGhost.getState().settings;
    if (s) useGhost.getState().setSettings({ ...s, simple: { ...s.simple, checked: true } } as Settings);
  };

  useEffect(() => {
    if (!connected) {
      sawTune.current = false;
      setPhase(null); // try again on the next connect
      return;
    }
    if (checked !== false || phase === "tune") return;
    if (!probed) {
      setPhase("probe");
      return;
    }
    if (blocked === 0) return finish();
    setPhase("tune");
    Service.StartAutotune().catch(finish);
  }, [connected, checked, probed, blocked, phase]);

  // Auto-tune reports progress by events; it is done when it stops running.
  useEffect(() => {
    if (phase !== "tune" || !autotune) return;
    if (autotune.running) sawTune.current = true;
    else if (sawTune.current && connected) finish();
  }, [autotune, phase, connected]);

  return phase;
}

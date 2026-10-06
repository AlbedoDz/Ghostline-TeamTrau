import { useEffect, useRef, useState } from "react";
import { Service, type Settings } from "../../app/api";
import { useGhost } from "../../app/store";

/** What the first-run tune is doing: checking the test sites, or tuning. */
export type FirstRunPhase = null | "probe" | "tune";

const tlsBlocked = (rs: { site: string; stage: string }[] | null) =>
  new Set((rs ?? []).filter((r) => r.stage === "tls").map((r) => r.site));

/**
 * useFirstRunTune runs once, on the first connect after installing: it
 * checks the test sites through Ghostline and, when some are blocked at TLS
 * twice, auto-tunes DPI bypass. Disconnecting mid-way leaves it for the next
 * connect. The Simple interface shows it as one more connect step.
 */
export function useFirstRunTune(status: string): FirstRunPhase {
  const checked = useGhost((s) => s.settings?.simple?.checked);
  const autotune = useGhost((s) => s.autotune);
  const [phase, setPhase] = useState<FirstRunPhase>(null);
  const active = useRef(false);
  const sawTune = useRef(false);
  const connected = status === "protected" || status === "degraded";

  const reset = () => {
    active.current = false;
    sawTune.current = false;
    setPhase(null);
  };
  const finish = () => {
    reset();
    void Service.MarkNetworkChecked().catch(() => {});
    const s = useGhost.getState().settings;
    if (s) useGhost.getState().setSettings({ ...s, simple: { ...s.simple, checked: true } } as Settings);
  };

  useEffect(() => {
    if (!connected) {
      if (active.current) reset(); // try again on the next connect
      return;
    }
    if (checked !== false || active.current) return;
    active.current = true;
    setPhase("probe");
    void (async () => {
      try {
        const first = tlsBlocked(await Service.ProbeNow());
        let blocked = false;
        if (first.size > 0) {
          const second = tlsBlocked(await Service.ProbeNow()); // one failure can be a fluke
          blocked = [...first].some((site) => second.has(site));
        }
        if (!active.current) return; // disconnected meanwhile
        if (!blocked) return finish();
        setPhase("tune");
        await Service.StartAutotune();
      } catch {
        if (active.current) finish();
      }
    })();
  }, [connected, checked]);

  // Auto-tune reports progress by events; it is done when it stops running.
  useEffect(() => {
    if (phase !== "tune" || !autotune) return;
    if (autotune.running) sawTune.current = true;
    else if (sawTune.current && active.current && connected) finish();
  }, [autotune, phase, connected]);

  return phase;
}

import { Service, type Settings } from "./api";
import { useGhost } from "./store";

/**
 * saveSettings applies a change optimistically and persists it. On a
 * validation error the previous settings come back and the message is
 * returned for inline display.
 */
export async function saveSettings(patch: (s: Settings) => Settings): Promise<string | null> {
  const prev = useGhost.getState().settings;
  if (!prev) return null;
  const next = patch(structuredClone(prev));
  useGhost.getState().setSettings(next);
  try {
    await Service.SaveSettings(next);
    return null;
  } catch (e) {
    useGhost.getState().setSettings(prev);
    return e instanceof Error ? e.message : String(e);
  }
}

/**
 * refreshSettings reloads settings from Go after a binding changed them
 * directly (Fake SNI, DNS server), so the UI's copy is never stale: every
 * later save sends the whole copy.
 */
export async function refreshSettings(): Promise<void> {
  const s = await Service.GetSettings();
  if (s) useGhost.getState().setSettings(s);
}

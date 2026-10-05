import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Service } from "./api";

type Strategy = { id: string; name: Record<string, string> | null };

/**
 * useStrategyName turns a running strategy id ("z-split", "light") into the
 * name shown to the user, in the UI language: the engine's own list first,
 * then the GoodbyeDPI preset translations, then the id itself.
 */
export function useStrategyName(engine: string | undefined, id: string | undefined): string {
  const { t, i18n } = useTranslation();
  const [list, setList] = useState<Strategy[]>([]);
  useEffect(() => {
    if (!engine) return;
    Service.DPIStrategies?.(engine)
      ?.then((l) => setList((l ?? []) as Strategy[]))
      .catch(() => setList([]));
  }, [engine]);
  if (!id) return "";
  const st = list.find((s) => s.id === id);
  return st?.name?.[i18n.language] || st?.name?.en || t(`dpi.presets.${id}`, id);
}

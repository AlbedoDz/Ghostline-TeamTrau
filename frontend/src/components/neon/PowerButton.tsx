import css from "./neon.module.css";

export type PowerState = "off" | "busy" | "on" | "err" | "warn";

type Props = { state: PowerState; size?: number; onClick: () => void; label: string; disabled?: boolean };

export function PowerButton({ state, size = 150, onClick, label, disabled }: Props) {
  return (
    <button
      className={css.power}
      data-state={state}
      aria-label={label}
      disabled={disabled}
      onClick={onClick}
      style={{ "--size": `${size}px` } as React.CSSProperties}
    >
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" aria-hidden>
        <path d="M12 3v8" />
        <path d="M6.3 6.3a8 8 0 1 0 11.4 0" />
      </svg>
    </button>
  );
}

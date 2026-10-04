import css from "./neon.module.css";

type Props = { checked: boolean; onChange: (v: boolean) => void; label: string; showLabel?: boolean; disabled?: boolean };

export function Toggle({ checked, onChange, label, showLabel, disabled }: Props) {
  return (
    <button
      role="switch"
      aria-checked={checked}
      aria-label={label}
      disabled={disabled}
      className={css.toggle}
      onClick={() => onChange(!checked)}
    >
      <span className={css.track} aria-hidden />
      {showLabel && <span>{label}</span>}
    </button>
  );
}

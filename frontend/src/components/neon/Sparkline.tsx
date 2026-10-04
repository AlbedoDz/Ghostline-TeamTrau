import { useId } from "react";

type Props = { points: number[]; width: number; height: number; colour?: string };

/** Sparkline draws values left→right scaled to the box; no chart library. */
export function Sparkline({ points, width, height, colour = "var(--green)" }: Props) {
  const id = useId();
  if (points.length === 0) return <svg width="100%" height={height} aria-hidden />;
  const max = Math.max(...points, 1);
  const step = points.length > 1 ? width / (points.length - 1) : 0;
  const coords = points.map((v, i) => `${(i * step).toFixed(1)},${(height - (v / max) * (height - 4) - 2).toFixed(1)}`);
  return (
    <svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="none" width="100%" height={height} aria-hidden>
      <defs>
        <linearGradient id={id} x1="0" x2="0" y1="0" y2="1">
          <stop offset="0" stopColor={colour} stopOpacity="0.35" />
          <stop offset="1" stopColor={colour} stopOpacity="0" />
        </linearGradient>
      </defs>
      <polygon fill={`url(#${id})`} points={`0,${height} ${coords.join(" ")} ${((points.length - 1) * step).toFixed(1)},${height}`} />
      <polyline fill="none" stroke={colour} strokeWidth="2" vectorEffect="non-scaling-stroke" points={coords.join(" ")} />
    </svg>
  );
}

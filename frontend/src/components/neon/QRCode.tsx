type Props = { matrix: boolean[][]; label: string; size?: number };

/** QRCode draws a module matrix from Go (internal/qr) as SVG, with a quiet zone. */
export function QRCode({ matrix, label, size = 160 }: Props) {
  const quiet = 4;
  const n = matrix.length + quiet * 2;
  let d = "";
  matrix.forEach((row, y) =>
    row.forEach((dark, x) => {
      if (dark) d += `M${x + quiet} ${y + quiet}h1v1h-1z`;
    }),
  );
  return (
    <svg role="img" aria-label={label} width={size} height={size} viewBox={`0 0 ${n} ${n}`} shapeRendering="crispEdges">
      <rect width={n} height={n} fill="#fff" />
      <path d={d} fill="#000" />
    </svg>
  );
}

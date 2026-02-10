import { createMemo } from "solid-js";

interface SparklineProps {
  values: number[];
  width?: number;
  height?: number;
  color?: string;
  class?: string;
  "aria-label"?: string;
}

export function Sparkline(props: SparklineProps) {
  const w = () => props.width ?? 120;
  const h = () => props.height ?? 32;
  const stroke = () => props.color ?? "var(--v2-accent)";

  const points = createMemo(() => {
    const vals = props.values;
    if (!vals || vals.length < 2) return "";
    const min = Math.min(...vals);
    const max = Math.max(...vals);
    const range = max - min || 1;
    const stepX = w() / (vals.length - 1);
    const pad = 2;
    const usableH = h() - pad * 2;
    return vals
      .map((v, i) => `${(i * stepX).toFixed(1)},${(pad + usableH - ((v - min) / range) * usableH).toFixed(1)}`)
      .join(" ");
  });

  const gradId = createMemo(() => `spark-${Math.random().toString(36).slice(2, 8)}`);

  return (
    <svg
      class={`sparkline-svg${props.class ? ` ${props.class}` : ""}`}
      width={w()}
      height={h()}
      viewBox={`0 0 ${w()} ${h()}`}
      preserveAspectRatio="none"
      role="img"
      aria-label={props["aria-label"] || "Sparkline"}
    >
      <defs>
        <linearGradient id={gradId()} x1="0" y1="0" x2="0" y2="1">
          <stop offset="0%" stop-color={stroke()} stop-opacity="0.25" />
          <stop offset="100%" stop-color={stroke()} stop-opacity="0" />
        </linearGradient>
      </defs>
      {points() && (
        <>
          <polygon
            points={`0,${h()} ${points()} ${w()},${h()}`}
            fill={`url(#${gradId()})`}
          />
          <polyline
            points={points()}
            fill="none"
            stroke={stroke()}
            stroke-width="1.5"
            stroke-linejoin="round"
            stroke-linecap="round"
          />
        </>
      )}
    </svg>
  );
}

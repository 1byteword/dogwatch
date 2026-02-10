import { createEffect, onCleanup, onMount } from "solid-js";
import uPlot from "uplot";
import "uplot/dist/uPlot.min.css";

interface SeriesConfig {
  label: string;
  stroke: string;
  fill?: string;
  width?: number;
}

interface ChartPanelProps {
  data: uPlot.AlignedData;
  series: SeriesConfig[];
  height?: number;
  class?: string;
  "aria-label"?: string;
}

const DEFAULT_SERIES_COLORS = ["#ccff00", "#2ed67a", "#ffc34d", "#ff4d55"];

export function ChartPanel(props: ChartPanelProps) {
  let containerRef!: HTMLDivElement;
  let chart: uPlot | undefined;

  function buildOpts(width: number): uPlot.Options {
    return {
      width,
      height: props.height ?? 200,
      cursor: {
        show: true,
        x: true,
        y: true,
      },
      legend: { show: false },
      axes: [
        {
          stroke: "#8a8a8a",
          grid: { stroke: "rgba(47,47,47,0.5)", width: 1 },
          ticks: { stroke: "rgba(47,47,47,0.5)", width: 1 },
          font: "11px 'IBM Plex Sans', sans-serif",
        },
        {
          stroke: "#8a8a8a",
          grid: { stroke: "rgba(47,47,47,0.5)", width: 1 },
          ticks: { stroke: "rgba(47,47,47,0.5)", width: 1 },
          font: "11px 'IBM Plex Sans', sans-serif",
          size: 50,
        },
      ],
      series: [
        {},
        ...props.series.map((s, i) => ({
          label: s.label,
          stroke: s.stroke || DEFAULT_SERIES_COLORS[i % DEFAULT_SERIES_COLORS.length],
          fill: s.fill,
          width: s.width ?? 2,
        })),
      ],
    };
  }

  onMount(() => {
    const width = containerRef.clientWidth || 400;
    chart = new uPlot(buildOpts(width), props.data, containerRef);

    const ro = new ResizeObserver((entries) => {
      for (const entry of entries) {
        const newWidth = entry.contentRect.width;
        if (newWidth > 0 && chart) {
          chart.setSize({ width: newWidth, height: props.height ?? 200 });
        }
      }
    });
    ro.observe(containerRef);

    onCleanup(() => {
      ro.disconnect();
      chart?.destroy();
      chart = undefined;
    });
  });

  createEffect(() => {
    const d = props.data;
    if (chart && d) {
      chart.setData(d);
    }
  });

  return (
    <div
      ref={containerRef}
      class={`chart-panel${props.class ? ` ${props.class}` : ""}`}
      role="img"
      aria-label={props["aria-label"] || "Chart"}
    />
  );
}

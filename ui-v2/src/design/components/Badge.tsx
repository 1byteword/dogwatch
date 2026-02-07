import { ParentProps } from "solid-js";

type BadgeTone = "neutral" | "ok" | "warn" | "error";

interface BadgeProps extends ParentProps {
  tone?: BadgeTone;
}

export function Badge(props: BadgeProps) {
  const tone = () => props.tone ?? "neutral";
  return <span class={`badge tone-${tone()}`}>{props.children}</span>;
}

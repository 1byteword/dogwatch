import { ParentProps } from "solid-js";

interface ChipProps extends ParentProps {}

export function Chip(props: ChipProps) {
  return <span class="context-chip">{props.children}</span>;
}

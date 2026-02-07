import { JSX, ParentProps } from "solid-js";

interface PanelProps extends ParentProps {
  title: string;
  actions?: JSX.Element;
}

export function Panel(props: PanelProps) {
  return (
    <section class="panel">
      <header class="panel-head">
        <h2>{props.title}</h2>
        <div class="panel-actions">{props.actions}</div>
      </header>
      <div class="panel-body">{props.children}</div>
    </section>
  );
}

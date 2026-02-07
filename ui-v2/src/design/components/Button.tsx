import { JSX } from "solid-js";

type ButtonVariant = "default" | "primary" | "danger";

interface ButtonProps extends JSX.ButtonHTMLAttributes<HTMLButtonElement> {
  variant?: ButtonVariant;
}

export function Button(props: ButtonProps) {
  const variant = () => props.variant ?? "default";
  return (
    <button
      {...props}
      class={`btn btn-${variant()}${props.class ? ` ${props.class}` : ""}`}
    >
      {props.children}
    </button>
  );
}

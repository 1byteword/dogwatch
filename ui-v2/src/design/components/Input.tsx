import { JSX } from "solid-js";

interface InputProps extends JSX.InputHTMLAttributes<HTMLInputElement> {}

export function Input(props: InputProps) {
  return <input {...props} class={`input${props.class ? ` ${props.class}` : ""}`} />;
}

import { Button } from "./Button";

interface WidgetErrorFallbackProps {
  error: Error;
  reset: () => void;
}

export function WidgetErrorFallback(props: WidgetErrorFallbackProps) {
  const message = () => {
    const msg = props.error?.message || "Unknown error";
    return msg.length > 200 ? msg.slice(0, 200) + "..." : msg;
  };

  return (
    <div class="widget-error" role="alert">
      <h4>Widget error</h4>
      <p>{message()}</p>
      <Button onClick={() => props.reset()}>Retry</Button>
    </div>
  );
}

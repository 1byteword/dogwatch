import { ErrorBoundary } from "solid-js";
import { A } from "@solidjs/router";
import { WidgetErrorFallback } from "../design/components/WidgetErrorFallback";

export function NotFoundPage() {
  return (
    <ErrorBoundary fallback={(err, reset) => <WidgetErrorFallback error={err} reset={reset} />}>
    <div class="not-found" style="display:flex;flex-direction:column;align-items:center;justify-content:center;min-height:60vh;gap:16px;">
      <h2 style="font-size:3rem;margin:0;color:var(--text-muted,#888);">404</h2>
      <p style="color:var(--text-muted,#888);margin:0;">Page not found</p>
      <A href="/app/dashboards" style="color:var(--accent,#ccff00);text-decoration:none;">
        Go to Dashboards
      </A>
    </div>
    </ErrorBoundary>
  );
}

export default NotFoundPage;

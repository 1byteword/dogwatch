import { A } from "@solidjs/router";

export function NotFoundPage() {
  return (
    <div class="not-found" style="display:flex;flex-direction:column;align-items:center;justify-content:center;min-height:60vh;gap:16px;">
      <h2 style="font-size:3rem;margin:0;color:var(--text-muted,#888);">404</h2>
      <p style="color:var(--text-muted,#888);margin:0;">Page not found</p>
      <A href="/app/dashboards" style="color:var(--accent,#ccff00);text-decoration:none;">
        Go to Dashboards
      </A>
    </div>
  );
}

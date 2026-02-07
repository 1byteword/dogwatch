import { Navigate, Route, Router } from "@solidjs/router";
import { AppShell } from "./shell/AppShell";
import { DetectAlertsPage } from "../routes/DetectAlertsPage";
import { InvestigateLogsPage } from "../routes/InvestigateLogsPage";
import { RespondIncidentsPage } from "../routes/RespondIncidentsPage";
import { CorrelateTimelinePage } from "../routes/CorrelateTimelinePage";
import { ImproveOncallPage } from "../routes/ImproveOncallPage";
import { ImproveKubernetesPage } from "../routes/ImproveKubernetesPage";
import { ConfigureCatalogPage } from "../routes/ConfigureCatalogPage";
import { ConfigureNotificationsPage } from "../routes/ConfigureNotificationsPage";
import { ConfigureAuditPage } from "../routes/ConfigureAuditPage";
import { StyleGuidePage } from "../routes/StyleGuidePage";
import { DashboardsPage } from "../routes/DashboardsPage";

export function AppRouter() {
  return (
    <Router root={AppShell}>
      <Route path="/" component={() => <Navigate href="/app/dashboards" />} />
      <Route path="/app/dashboards" component={DashboardsPage} />
      <Route path="/app/detect/alerts" component={DetectAlertsPage} />
      <Route path="/app/investigate/logs" component={InvestigateLogsPage} />
      <Route path="/app/correlate/timeline" component={CorrelateTimelinePage} />
      <Route path="/app/respond/incidents" component={RespondIncidentsPage} />
      <Route path="/app/improve/oncall" component={ImproveOncallPage} />
      <Route path="/app/improve/kubernetes" component={ImproveKubernetesPage} />
      <Route path="/app/configure/catalog" component={ConfigureCatalogPage} />
      <Route path="/app/configure/notifications" component={ConfigureNotificationsPage} />
      <Route path="/app/configure/audit" component={ConfigureAuditPage} />
      <Route path="/app/style-guide" component={StyleGuidePage} />
    </Router>
  );
}

import { lazy } from "solid-js";
import { Navigate, Route, Router } from "@solidjs/router";
import { AppShell } from "./shell/AppShell";

const DashboardsPage = lazy(() => import("../routes/DashboardsPage"));
const DetectAlertsPage = lazy(() => import("../routes/DetectAlertsPage"));
const MonitorsPage = lazy(() => import("../routes/MonitorsPage"));
const InvestigateLogsPage = lazy(() => import("../routes/InvestigateLogsPage"));
const QueryExplorerPage = lazy(() => import("../routes/QueryExplorerPage"));
const CorrelateTimelinePage = lazy(() => import("../routes/CorrelateTimelinePage"));
const RespondIncidentsPage = lazy(() => import("../routes/RespondIncidentsPage"));
const ImproveOncallPage = lazy(() => import("../routes/ImproveOncallPage"));
const ImproveKubernetesPage = lazy(() => import("../routes/ImproveKubernetesPage"));
const ConfigureCatalogPage = lazy(() => import("../routes/ConfigureCatalogPage"));
const ConfigureNotificationsPage = lazy(() => import("../routes/ConfigureNotificationsPage"));
const ConfigureAuditPage = lazy(() => import("../routes/ConfigureAuditPage"));
const SloManagementPage = lazy(() => import("../routes/SloManagementPage"));
const SyntheticsManagementPage = lazy(() => import("../routes/SyntheticsManagementPage"));
const RecordingRulesPage = lazy(() => import("../routes/RecordingRulesPage"));
const StyleGuidePage = lazy(() => import("../routes/StyleGuidePage"));
const NotFoundPage = lazy(() => import("../routes/NotFoundPage"));

export function AppRouter() {
  return (
    <Router root={AppShell}>
      <Route path="/" component={() => <Navigate href="/app/dashboards" />} />
      <Route path="/app/dashboards" component={DashboardsPage} />
      <Route path="/app/detect/alerts" component={DetectAlertsPage} />
      <Route path="/app/detect/monitors" component={MonitorsPage} />
      <Route path="/app/investigate/logs" component={InvestigateLogsPage} />
      <Route path="/app/explore/query" component={QueryExplorerPage} />
      <Route path="/app/correlate/timeline" component={CorrelateTimelinePage} />
      <Route path="/app/respond/incidents" component={RespondIncidentsPage} />
      <Route path="/app/improve/oncall" component={ImproveOncallPage} />
      <Route path="/app/improve/kubernetes" component={ImproveKubernetesPage} />
      <Route path="/app/configure/catalog" component={ConfigureCatalogPage} />
      <Route path="/app/configure/notifications" component={ConfigureNotificationsPage} />
      <Route path="/app/configure/audit" component={ConfigureAuditPage} />
      <Route path="/app/configure/slos" component={SloManagementPage} />
      <Route path="/app/configure/synthetics" component={SyntheticsManagementPage} />
      <Route path="/app/configure/recording-rules" component={RecordingRulesPage} />
      <Route path="/app/style-guide" component={StyleGuidePage} />
      <Route path="*" component={NotFoundPage} />
    </Router>
  );
}

import { expect, test } from '@playwright/test';
import { gotoV2, expectPanel } from './helpers';

/** Routes with a `.panel-head h2` heading use expectPanel; others have a custom check. */
const panelRoutes: Array<{ path: string; heading: RegExp }> = [
  { path: '/app/detect/alerts', heading: /Alert Feed/i },
  { path: '/app/detect/monitors', heading: /Monitor/i },
  { path: '/app/investigate/logs', heading: /Logs Explorer/i },
  { path: '/app/explore/query', heading: /Query/i },
  { path: '/app/correlate/timeline', heading: /Correlat/i },
  { path: '/app/respond/incidents', heading: /Incidents/i },
  { path: '/app/improve/oncall', heading: /On-call/i },
  { path: '/app/improve/kubernetes', heading: /Cluster/i },
  { path: '/app/configure/catalog', heading: /Service Catalog/i },
  { path: '/app/configure/notifications', heading: /Notification/i },
  { path: '/app/configure/audit', heading: /Audit/i },
  { path: '/app/configure/slos', heading: /SLO/i },
  { path: '/app/configure/synthetics', heading: /Synthetic/i },
  { path: '/app/configure/recording-rules', heading: /Recording Rule/i },
];

test.describe('V2 route smoke tests', () => {
  for (const { path, heading } of panelRoutes) {
    test(`${path} renders`, async ({ page }) => {
      await gotoV2(page, path);
      await expectPanel(page, heading);
    });
  }

  test('/app/dashboards renders', async ({ page }) => {
    await gotoV2(page, '/app/dashboards');
    await expect(page.locator('.dashboard-toolbar')).toBeVisible();
  });

  test('/app/style-guide renders', async ({ page }) => {
    await gotoV2(page, '/app/style-guide');
    await expect(page.getByRole('heading', { name: /Buttons/i })).toBeVisible();
  });
});

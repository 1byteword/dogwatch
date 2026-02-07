import { expect, test } from '@playwright/test';

async function gotoRoute(page: import('@playwright/test').Page, path: string, heading: RegExp) {
  await page.goto(path);
  await expect(page.locator('.app-shell')).toBeVisible();
  await expect(page.locator('.panel-head h2').first()).toContainText(heading);
}

test.describe('UI V2 smoke', () => {
  test('shell command navigation works', async ({ page }) => {
    await page.goto('/app/detect/alerts');
    const command = page.getByPlaceholder(/Command/i);
    await command.fill('audit');
    await page.getByRole('button', { name: 'Run' }).click();
    await expect(page).toHaveURL(/\/app\/configure\/audit/);
  });

  test('core routes render', async ({ page }) => {
    await gotoRoute(page, '/app/detect/alerts', /Alert Feed/i);
    await gotoRoute(page, '/app/investigate/logs', /Logs Explorer/i);
    await gotoRoute(page, '/app/correlate/timeline', /Deploy -> Incident Correlations/i);
    await gotoRoute(page, '/app/respond/incidents', /Incidents/i);
    await gotoRoute(page, '/app/improve/oncall', /On-call Schedules/i);
    await gotoRoute(page, '/app/improve/kubernetes', /Cluster Summary/i);
    await gotoRoute(page, '/app/configure/catalog', /Service Catalog/i);
    await gotoRoute(page, '/app/configure/notifications', /Notification Channels/i);
    await gotoRoute(page, '/app/configure/audit', /Audit Summary/i);
  });

  test('detect workflow modal opens', async ({ page }) => {
    await page.goto('/app/detect/alerts');
    await page.getByRole('button', { name: 'New Rule' }).click();
    await expect(page.getByRole('heading', { name: /Create Alert Rule/i })).toBeVisible();
    await page.getByRole('button', { name: 'Cancel' }).click();
    await expect(page.getByRole('heading', { name: /Create Alert Rule/i })).toBeHidden();
  });

  test('catalog import modal opens', async ({ page }) => {
    await page.goto('/app/configure/catalog');
    await page.getByRole('button', { name: 'Import' }).click();
    await expect(page.getByRole('heading', { name: /Import Services/i })).toBeVisible();
  });

  test('notifications create modal opens', async ({ page }) => {
    await page.goto('/app/configure/notifications');
    await page.getByRole('button', { name: 'New Channel' }).click();
    await expect(page.getByRole('heading', { name: /New Notification Channel/i })).toBeVisible();
  });
});

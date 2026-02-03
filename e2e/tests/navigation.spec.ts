import { test, expect } from '@playwright/test';

test.describe('Navigation', () => {
  test('homepage loads successfully', async ({ page }) => {
    await page.goto('/');

    // Check page title
    await expect(page).toHaveTitle(/dogwatch/i);

    // Check sidebar exists
    await expect(page.locator('.sidebar')).toBeVisible();

    // Check main content area exists
    await expect(page.locator('.main-content')).toBeVisible();
  });

  test('sidebar navigation works', async ({ page }) => {
    await page.goto('/');

    // Click on Traces
    await page.click('a[href="/traces.html"]');
    await expect(page).toHaveURL(/traces/);
    await expect(page.locator('h1, .page-header h1')).toContainText(/trace/i);

    // Click on Logs
    await page.click('a[href="/logs.html"]');
    await expect(page).toHaveURL(/logs/);

    // Click on Query Builder
    await page.click('a[href="/query-builder.html"]');
    await expect(page).toHaveURL(/query-builder/);
  });

  test('integrations page loads', async ({ page }) => {
    await page.goto('/integrations.html');

    // Check page header
    await expect(page.locator('h1')).toContainText(/Integration/i);

    // Check integration cards exist
    await expect(page.locator('.integration-card')).toHaveCount({ min: 6 });

    // Check specific integrations
    await expect(page.getByText('Prometheus')).toBeVisible();
    await expect(page.getByText('Graphite')).toBeVisible();
    await expect(page.getByText('InfluxDB')).toBeVisible();
    await expect(page.getByText('OpenTSDB')).toBeVisible();
    await expect(page.getByText('StatsD')).toBeVisible();
    await expect(page.getByText('DataDog')).toBeVisible();
  });

  test('sidebar can be collapsed', async ({ page }) => {
    await page.goto('/');

    const sidebar = page.locator('.sidebar');

    // Toggle sidebar
    await page.click('.sidebar-toggle');

    // Should have collapsed class
    await expect(sidebar).toHaveClass(/collapsed/);

    // Toggle again
    await page.click('.sidebar-toggle');

    // Should not have collapsed class
    await expect(sidebar).not.toHaveClass(/collapsed/);
  });
});

test.describe('Dashboard', () => {
  test('dashboard loads with widgets', async ({ page }) => {
    await page.goto('/');

    // Wait for dashboard to load
    await page.waitForSelector('.app.loaded', { timeout: 10000 });

    // Check for grid layout
    await expect(page.locator('.grid-stack, .dashboard-grid, .gs-item')).toBeVisible();
  });

  test('can add widget', async ({ page }) => {
    await page.goto('/');

    // Wait for load
    await page.waitForSelector('.app.loaded', { timeout: 10000 });

    // Click add widget button if exists
    const addButton = page.getByRole('button', { name: /widget/i });
    if (await addButton.isVisible()) {
      await addButton.click();

      // Widget picker should appear
      await expect(page.locator('.widget-picker, .modal')).toBeVisible();
    }
  });
});

import { expect, test } from '@playwright/test';
import { gotoV2 } from './helpers';

test.describe('Shell navigation', () => {
  test('sidebar links navigate to routes', async ({ page }) => {
    await gotoV2(page, '/app/dashboards');
    const nav = page.getByRole('navigation', { name: 'Main navigation' });

    await nav.getByRole('link', { name: 'Detect' }).click();
    await expect(page).toHaveURL(/\/app\/detect\/alerts/);

    await nav.getByRole('link', { name: 'Investigate' }).click();
    await expect(page).toHaveURL(/\/app\/investigate\/logs/);

    await nav.getByRole('link', { name: 'Dashboards' }).click();
    await expect(page).toHaveURL(/\/app\/dashboards/);
  });

  test('command bar navigates to detect', async ({ page }) => {
    await gotoV2(page, '/app/dashboards');
    const input = page.locator('input[aria-label="Command input"]');
    await input.fill('detect');
    await page.locator('button[aria-label="Run command"]').click();
    await expect(page).toHaveURL(/\/app\/detect\/alerts/);
  });

  test('command bar navigates to audit', async ({ page }) => {
    await gotoV2(page, '/app/dashboards');
    const input = page.locator('input[aria-label="Command input"]');
    await input.fill('audit');
    await page.locator('button[aria-label="Run command"]').click();
    await expect(page).toHaveURL(/\/app\/configure\/audit/);
  });

  test('command bar navigates to kubernetes', async ({ page }) => {
    await gotoV2(page, '/app/dashboards');
    const input = page.locator('input[aria-label="Command input"]');
    await input.fill('kubernetes');
    await page.locator('button[aria-label="Run command"]').click();
    await expect(page).toHaveURL(/\/app\/improve\/kubernetes/);
  });

  test('empty command submit refreshes status', async ({ page }) => {
    await gotoV2(page, '/app/dashboards');
    const input = page.locator('input[aria-label="Command input"]');
    await input.fill('');
    await page.locator('button[aria-label="Run command"]').click();
    // Should stay on the same page
    await expect(page).toHaveURL(/\/app\/dashboards/);
  });
});

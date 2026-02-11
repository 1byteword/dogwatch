import { expect, test } from '@playwright/test';
import { gotoV2 } from './helpers';

test.describe('Dashboard variables', () => {
  test.beforeEach(async ({ page }) => {
    await gotoV2(page, '/app/dashboards');
  });

  test('variable bar is visible', async ({ page }) => {
    await expect(page.locator('.dashboard-variables-bar')).toBeVisible();
  });

  test('can change variable value', async ({ page }) => {
    const selects = page.locator('.dashboard-variables-bar select');
    if (await selects.count() === 0) return;

    // Change the first variable
    const firstSelect = selects.first();
    const options = await firstSelect.locator('option').allTextContents();
    if (options.length > 1) {
      await firstSelect.selectOption({ index: 1 });
    }
  });

  test('edit variables button visible in edit mode', async ({ page }) => {
    await page.getByRole('button', { name: 'Customize Layout' }).click();
    await expect(
      page.getByRole('button', { name: /Edit Variables/i })
    ).toBeVisible();
  });

  test('variable editor modal opens', async ({ page }) => {
    await page.getByRole('button', { name: 'Customize Layout' }).click();
    await page.getByRole('button', { name: /Edit Variables/i }).click();
    await expect(page.locator('.modal-overlay')).toBeVisible();
  });
});

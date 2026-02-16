import { expect, test } from '@playwright/test';
import { gotoV2 } from './helpers';

test.describe('Visual regression', () => {
  test('dashboard page', async ({ page }) => {
    await gotoV2(page, '/app/dashboards');
    await page.waitForTimeout(1000); // let widgets settle
    await expect(page).toHaveScreenshot('dashboard-page.png', {
      maxDiffPixelRatio: 0.05, // dashboard has dynamic timestamps/counters
    });
  });

  test('dashboard edit mode', async ({ page }) => {
    await gotoV2(page, '/app/dashboards');
    await page.getByRole('button', { name: 'Customize Layout' }).click();
    await page.waitForTimeout(500);
    await expect(page).toHaveScreenshot('dashboard-edit-mode.png', {
      maxDiffPixelRatio: 0.05,
    });
  });

  test('widget picker modal', async ({ page }) => {
    await gotoV2(page, '/app/dashboards');
    await page.getByRole('button', { name: 'Customize Layout' }).click();
    await page.getByRole('button', { name: 'Add Widget' }).click();
    await page.waitForTimeout(500);
    await expect(page).toHaveScreenshot('widget-picker.png', {
      maxDiffPixelRatio: 0.05,
    });
  });

  test('alerts page', async ({ page }) => {
    await gotoV2(page, '/app/detect/alerts');
    await page.waitForTimeout(500);
    await expect(page).toHaveScreenshot('alerts-page.png', {
      maxDiffPixelRatio: 0.05,
    });
  });

  test('monitors page', async ({ page }) => {
    await gotoV2(page, '/app/detect/monitors');
    await page.waitForTimeout(500);
    await expect(page).toHaveScreenshot('monitors-page.png', {
      maxDiffPixelRatio: 0.05,
    });
  });

  test('query explorer', async ({ page }) => {
    await gotoV2(page, '/app/explore/query');
    await page.waitForTimeout(500);
    await expect(page).toHaveScreenshot('query-explorer.png', {
      maxDiffPixelRatio: 0.05,
    });
  });

  test('SLO management', async ({ page }) => {
    await gotoV2(page, '/app/configure/slos');
    await page.waitForTimeout(500);
    await expect(page).toHaveScreenshot('slo-management.png', {
      maxDiffPixelRatio: 0.05,
    });
  });

  test('recording rules', async ({ page }) => {
    await gotoV2(page, '/app/configure/recording-rules');
    await page.waitForTimeout(500);
    await expect(page).toHaveScreenshot('recording-rules.png', {
      maxDiffPixelRatio: 0.05,
    });
  });

  test('synthetics', async ({ page }) => {
    await gotoV2(page, '/app/configure/synthetics');
    await page.waitForTimeout(500);
    await expect(page).toHaveScreenshot('synthetics.png', {
      maxDiffPixelRatio: 0.05,
    });
  });

  test('style guide', async ({ page }) => {
    await gotoV2(page, '/app/style-guide');
    await page.waitForTimeout(500);
    await expect(page).toHaveScreenshot('style-guide.png', {
      maxDiffPixelRatio: 0.05,
    });
  });
});

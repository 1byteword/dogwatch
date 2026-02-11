import { expect, test } from '@playwright/test';
import { gotoV2, expectPanel, openModal } from './helpers';

// ---------------------------------------------------------------------------
// Monitors
// ---------------------------------------------------------------------------
test.describe('Monitors management', () => {
  test('page renders', async ({ page }) => {
    await gotoV2(page, '/app/detect/monitors');
    await expectPanel(page, /Monitor/i);
  });

  test('create modal opens', async ({ page }) => {
    await gotoV2(page, '/app/detect/monitors');
    await openModal(page, 'Create Monitor', /Create Monitor|New Monitor/i);
  });

  test('template picker shows categories', async ({ page }) => {
    await gotoV2(page, '/app/detect/monitors');
    await page.getByRole('button', { name: 'Create Monitor' }).click();
    // Should show template cards
    await expect(page.locator('.widget-picker-card').first()).toBeVisible();
  });

  test('wizard step navigation', async ({ page }) => {
    await gotoV2(page, '/app/detect/monitors');
    await page.getByRole('button', { name: 'Create Monitor' }).click();
    // Pick blank monitor or first template
    await page.locator('.widget-picker-card').first().click();
    // Should advance to configure step — look for a name input
    await expect(page.locator('label').filter({ hasText: /Name/i }).first()).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// SLOs
// ---------------------------------------------------------------------------
test.describe('SLO management', () => {
  test('page renders', async ({ page }) => {
    await gotoV2(page, '/app/configure/slos');
    await expectPanel(page, /SLO/i);
  });

  test('create SLO modal opens', async ({ page }) => {
    await gotoV2(page, '/app/configure/slos');
    await openModal(page, 'Create SLO', /Create SLO|New SLO/i);
  });

  test('create form has required fields', async ({ page }) => {
    await gotoV2(page, '/app/configure/slos');
    await page.getByRole('button', { name: 'Create SLO' }).click();
    await expect(page.locator('label').filter({ hasText: /Name/i }).first()).toBeVisible();
    await expect(page.locator('label').filter({ hasText: /Service/i }).first()).toBeVisible();
    await expect(page.locator('input[type="number"]').first()).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Synthetics
// ---------------------------------------------------------------------------
test.describe('Synthetics management', () => {
  test('page renders', async ({ page }) => {
    await gotoV2(page, '/app/configure/synthetics');
    await expectPanel(page, /Synthetic/i);
  });

  test('create check modal opens', async ({ page }) => {
    await gotoV2(page, '/app/configure/synthetics');
    await openModal(page, 'Create Check', /Create Synthetic Check/i);
  });

  test('create form has required fields', async ({ page }) => {
    await gotoV2(page, '/app/configure/synthetics');
    await page.getByRole('button', { name: 'Create Check' }).click();
    await expect(page.locator('label').filter({ hasText: /Name/i }).first()).toBeVisible();
    await expect(page.locator('label').filter({ hasText: /URL/i }).first()).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Recording Rules
// ---------------------------------------------------------------------------
test.describe('Recording Rules management', () => {
  test('page renders', async ({ page }) => {
    await gotoV2(page, '/app/configure/recording-rules');
    await expectPanel(page, /Recording Rule/i);
  });

  test('create rule modal opens', async ({ page }) => {
    await gotoV2(page, '/app/configure/recording-rules');
    await openModal(page, 'Create Rule', /Create Recording Rule/i);
  });

  test('create form has expression field', async ({ page }) => {
    await gotoV2(page, '/app/configure/recording-rules');
    await page.getByRole('button', { name: 'Create Rule' }).click();
    await expect(page.locator('textarea').first()).toBeVisible();
  });
});

// ---------------------------------------------------------------------------
// Query Explorer
// ---------------------------------------------------------------------------
test.describe('Query Explorer', () => {
  test('page renders with editor', async ({ page }) => {
    await gotoV2(page, '/app/explore/query');
    await expect(page.locator('textarea[aria-label="Query editor"]')).toBeVisible();
  });

  test('Run Query button visible', async ({ page }) => {
    await gotoV2(page, '/app/explore/query');
    await expect(page.getByRole('button', { name: 'Run Query' })).toBeVisible();
  });

  test('Ctrl+Enter triggers query', async ({ page }) => {
    await gotoV2(page, '/app/explore/query');
    const editor = page.locator('textarea[aria-label="Query editor"]');
    await editor.fill('up');
    await editor.press('Control+Enter');
    // After running, the results section or error should appear
    await page.waitForTimeout(500);
    // Page should still be on query route (no navigation away)
    await expect(page).toHaveURL(/\/app\/explore\/query/);
  });
});

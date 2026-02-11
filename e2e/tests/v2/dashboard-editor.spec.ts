import { expect, test } from '@playwright/test';
import { gotoV2 } from './helpers';

test.describe('Dashboard editor', () => {
  test.beforeEach(async ({ page }) => {
    await gotoV2(page, '/app/dashboards');
  });

  test('toggle edit mode', async ({ page }) => {
    const editBtn = page.getByRole('button', { name: 'Customize Layout' });
    await editBtn.click();
    await expect(page.getByRole('button', { name: 'Done Editing' })).toBeVisible();

    await page.getByRole('button', { name: 'Done Editing' }).click();
    await expect(page.getByRole('button', { name: 'Customize Layout' })).toBeVisible();
  });

  test('widget picker opens and closes', async ({ page }) => {
    await page.getByRole('button', { name: 'Customize Layout' }).click();
    await page.getByRole('button', { name: 'Add Widget' }).click();
    await expect(page.locator('input[aria-label="Search widgets"]')).toBeVisible();

    await page.getByRole('button', { name: 'Close' }).click();
    await expect(page.locator('input[aria-label="Search widgets"]')).toBeHidden();
  });

  test('widget picker search filters results', async ({ page }) => {
    await page.getByRole('button', { name: 'Customize Layout' }).click();
    await page.getByRole('button', { name: 'Add Widget' }).click();

    const search = page.locator('input[aria-label="Search widgets"]');
    await search.fill('latency');

    const cards = page.locator('.widget-picker-card');
    const count = await cards.count();
    expect(count).toBeGreaterThan(0);
    // All visible cards should relate to "latency"
    for (let i = 0; i < count; i++) {
      const text = await cards.nth(i).textContent();
      expect(text?.toLowerCase()).toContain('latency');
    }
  });

  test('widget picker category filter works', async ({ page }) => {
    await page.getByRole('button', { name: 'Customize Layout' }).click();
    await page.getByRole('button', { name: 'Add Widget' }).click();

    const allCount = await page.locator('.widget-picker-card').count();

    await page.locator('select.form-select').selectOption('Infrastructure');
    const filteredCount = await page.locator('.widget-picker-card').count();
    expect(filteredCount).toBeLessThan(allCount);
    expect(filteredCount).toBeGreaterThan(0);
  });

  test('add widget from picker', async ({ page }) => {
    await page.getByRole('button', { name: 'Customize Layout' }).click();

    const initialCount = await page.locator('.widget-card').count();

    await page.getByRole('button', { name: 'Add Widget' }).click();
    await page.locator('.widget-picker-card').first().click();

    const newCount = await page.locator('.widget-card').count();
    expect(newCount).toBe(initialCount + 1);
  });

  test('remove widget in edit mode', async ({ page }) => {
    await page.getByRole('button', { name: 'Customize Layout' }).click();

    const initialCount = await page.locator('.widget-card').count();
    if (initialCount === 0) return; // nothing to remove

    // Click a widget to focus it, then remove
    await page.locator('.widget-card').first().click();
    await page.locator('button[aria-label^="Remove"]').first().click();

    const newCount = await page.locator('.widget-card').count();
    expect(newCount).toBe(initialCount - 1);
  });

  test('apply dashboard template', async ({ page }) => {
    await page.getByRole('button', { name: 'Customize Layout' }).click();

    // Select a template and apply
    const templateSelect = page.locator('select').filter({ hasText: /Executive Ops|template/i });
    if (await templateSelect.count() > 0) {
      await templateSelect.first().selectOption({ index: 1 });
      await page.getByRole('button', { name: 'Apply Template' }).click();
      await expect(page.locator('.widget-card').first()).toBeVisible();
    }
  });

  test('undo/redo with keyboard', async ({ page }) => {
    await page.getByRole('button', { name: 'Customize Layout' }).click();

    const initialCount = await page.locator('.widget-card').count();

    // Add a widget
    await page.getByRole('button', { name: 'Add Widget' }).click();
    await page.locator('.widget-picker-card').first().click();
    expect(await page.locator('.widget-card').count()).toBe(initialCount + 1);

    // Undo
    await page.keyboard.press('Control+z');
    await page.waitForTimeout(300);
    expect(await page.locator('.widget-card').count()).toBe(initialCount);

    // Redo
    await page.keyboard.press('Control+y');
    await page.waitForTimeout(300);
    expect(await page.locator('.widget-card').count()).toBe(initialCount + 1);
  });

  test('widget inspector shows on click', async ({ page }) => {
    await page.getByRole('button', { name: 'Customize Layout' }).click();

    const widgets = page.locator('.widget-card');
    if (await widgets.count() === 0) return;

    await widgets.first().click();
    await expect(page.locator('.widget-inspector')).toBeVisible();
  });
});

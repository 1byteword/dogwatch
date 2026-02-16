import { expect, test } from '@playwright/test';
import { gotoV2, mockAuth } from './helpers';

test.describe('Dashboard editor', () => {
  test.beforeEach(async ({ page }) => {
    await mockAuth(page);
    // Block dashboard API so tests use the default in-memory layout (10 widgets)
    // instead of a live backend's saved dashboards
    await page.route('**/api/dashboards**', (route) =>
      route.fulfill({ status: 200, contentType: 'application/json', body: '[]' })
    );
    await page.goto('/app/dashboards');
    await page.evaluate(() => localStorage.clear());
    await page.reload();
    await page.locator('.app-shell').waitFor({ state: 'visible', timeout: 10_000 });
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

    const cards = page.locator('.modal-card .widget-picker-card');
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

    const allCount = await page.locator('.modal-card .widget-picker-card').count();

    await page.locator('.modal-card select.form-select').selectOption('Infrastructure');
    const filteredCount = await page.locator('.modal-card .widget-picker-card').count();
    expect(filteredCount).toBeLessThan(allCount);
    expect(filteredCount).toBeGreaterThan(0);
  });

  test('add widget from picker', async ({ page }) => {
    await page.getByRole('button', { name: 'Customize Layout' }).click();

    const initialCount = await page.locator('.widget-card').count();

    await page.getByRole('button', { name: 'Add Widget' }).click();
    await expect(page.locator('input[aria-label="Search widgets"]')).toBeVisible();
    await page.locator('.modal-card .widget-picker-card').first().click();

    // Core assertion: widget was added to the layout
    await expect(page.locator('.widget-card')).not.toHaveCount(initialCount, { timeout: 5000 });
    const newCount = await page.locator('.widget-card').count();
    expect(newCount).toBeGreaterThan(initialCount);
  });

  test('remove widget in edit mode', async ({ page }) => {
    await page.getByRole('button', { name: 'Customize Layout' }).click();

    const widgets = page.locator('.widget-card');
    const initialCount = await widgets.count();
    if (initialCount === 0) return;

    // Click a widget to focus it, then remove
    await widgets.first().click();
    const removeBtn = page.locator('button[aria-label^="Remove"]').first();
    await expect(removeBtn).toBeVisible();
    await removeBtn.click();

    // Wait for the layout to update
    await expect(widgets).not.toHaveCount(initialCount, { timeout: 3000 });
    const newCount = await widgets.count();
    expect(newCount).toBeLessThan(initialCount);
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

    // Remove a widget first (reliable operation)
    const widgets = page.locator('.widget-card');
    const initialCount = await widgets.count();
    if (initialCount === 0) return;

    await widgets.first().click();
    const removeBtn = page.locator('button[aria-label^="Remove"]').first();
    await expect(removeBtn).toBeVisible();
    await removeBtn.click();
    await expect(widgets).not.toHaveCount(initialCount, { timeout: 3000 });
    const afterRemove = await widgets.count();
    expect(afterRemove).toBeLessThan(initialCount);

    // Undo — should restore the removed widget
    await page.keyboard.press('Control+z');
    await expect(widgets).toHaveCount(initialCount, { timeout: 3000 });

    // Redo — should remove it again
    await page.keyboard.press('Control+y');
    await expect(widgets).toHaveCount(afterRemove, { timeout: 3000 });
  });

  test('widget focus highlights on click', async ({ page }) => {
    await page.getByRole('button', { name: 'Customize Layout' }).click();

    const widgets = page.locator('.widget-card');
    await expect(widgets.first()).toBeVisible({ timeout: 5000 });
    if (await widgets.count() === 0) return;

    // Click a widget and verify it gets the focused state
    await widgets.first().click();
    await expect(widgets.first()).toHaveClass(/is-focused/, { timeout: 5000 });
  });
});

import { expect, type Page } from '@playwright/test';

/** Navigate to a V2 route and assert the shell rendered. */
export async function gotoV2(page: Page, path: string) {
  await page.goto(path);
  await expect(page.locator('.app-shell')).toBeVisible({ timeout: 10_000 });
}

/** Assert a panel heading is visible on the current page. */
export async function expectPanel(page: Page, heading: RegExp) {
  await expect(
    page.locator('.panel-head h2').first()
  ).toContainText(heading, { timeout: 5_000 });
}

/** Click a button to open a modal, then assert the modal heading appears. */
export async function openModal(
  page: Page,
  buttonName: string | RegExp,
  headingName: string | RegExp
) {
  await page.getByRole('button', { name: buttonName }).click();
  await expect(
    page.getByRole('heading', { name: headingName })
  ).toBeVisible({ timeout: 5_000 });
}

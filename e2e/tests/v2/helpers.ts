import { expect, type Page } from '@playwright/test';

const MOCK_ME_RESPONSE = {
  user: {
    id: 'e2e-user',
    email: 'test@dogwatch.io',
    name: 'E2E Tester',
    role: 'admin',
    isActive: true,
  },
};

/** Mock the auth endpoint so the auth guard lets pages through. */
export async function mockAuth(page: Page) {
  await page.route('**/api/auth/me', (route) =>
    route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify(MOCK_ME_RESPONSE) })
  );
}

/** Navigate to a V2 route and assert the shell rendered. */
export async function gotoV2(page: Page, path: string) {
  await mockAuth(page);
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

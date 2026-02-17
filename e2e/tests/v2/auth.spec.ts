import { expect, test } from '@playwright/test';

const MOCK_USER = {
  id: 'e2e-user',
  email: 'test@dogwatch.io',
  name: 'E2E Tester',
  role: 'admin',
  isActive: true,
};

test.describe('Authentication', () => {
  test('unauthenticated user is redirected to /login', async ({ page }) => {
    // Mock auth endpoint to return 401 (not authenticated)
    await page.route('**/api/auth/me', (route) =>
      route.fulfill({ status: 401, body: 'Unauthorized' })
    );

    await page.goto('/app/dashboards');

    // Should redirect to login page
    await expect(page).toHaveURL(/\/login/, { timeout: 10_000 });
    await expect(page.locator('.login-card')).toBeVisible();
  });

  test('login page renders correctly', async ({ page }) => {
    await page.route('**/api/auth/me', (route) =>
      route.fulfill({ status: 401, body: 'Unauthorized' })
    );

    await page.goto('/login');
    await expect(page.locator('.login-card')).toBeVisible({ timeout: 10_000 });

    // Logo and branding
    await expect(page.locator('.login-logo')).toBeVisible();
    await expect(page.locator('h1')).toContainText('dogwatch');

    // Form fields
    await expect(page.locator('#login-email')).toBeVisible();
    await expect(page.locator('#login-password')).toBeVisible();
    await expect(page.locator('.login-submit')).toBeVisible();
    await expect(page.locator('.login-submit')).toContainText('Sign in');
  });

  test('login with invalid credentials shows error', async ({ page }) => {
    await page.route('**/api/auth/me', (route) =>
      route.fulfill({ status: 401, body: 'Unauthorized' })
    );
    await page.route('**/api/auth/login', (route) =>
      route.fulfill({ status: 401, body: 'Invalid email or password' })
    );

    await page.goto('/login');
    await expect(page.locator('.login-card')).toBeVisible({ timeout: 10_000 });

    await page.locator('#login-email').fill('bad@example.com');
    await page.locator('#login-password').fill('wrongpass');
    await page.locator('.login-submit').click();

    // Error message should appear
    await expect(page.locator('.login-error')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('.login-error')).toContainText('Invalid email or password');
  });

  test('successful login redirects to dashboards', async ({ page }) => {
    // Initially not authenticated
    let authenticated = false;

    await page.route('**/api/auth/me', (route) => {
      if (authenticated) {
        return route.fulfill({
          status: 200,
          contentType: 'application/json',
          body: JSON.stringify({ user: MOCK_USER }),
        });
      }
      return route.fulfill({ status: 401, body: 'Unauthorized' });
    });

    await page.route('**/api/auth/login', (route) => {
      authenticated = true;
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          token: 'mock-token',
          expires_at: new Date(Date.now() + 86400000).toISOString(),
          user: MOCK_USER,
        }),
      });
    });

    await page.goto('/login');
    await expect(page.locator('.login-card')).toBeVisible({ timeout: 10_000 });

    await page.locator('#login-email').fill('test@dogwatch.io');
    await page.locator('#login-password').fill('password123');
    await page.locator('.login-submit').click();

    // Should redirect to dashboards after login
    await expect(page).toHaveURL(/\/app\/dashboards/, { timeout: 10_000 });
    await expect(page.locator('.app-shell')).toBeVisible({ timeout: 10_000 });
  });

  test('authenticated user sees shell with user info', async ({ page }) => {
    await page.route('**/api/auth/me', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ user: MOCK_USER }),
      })
    );

    await page.goto('/app/dashboards');
    await expect(page.locator('.app-shell')).toBeVisible({ timeout: 10_000 });

    // User name and sign-out button should be in the topbar
    await expect(page.locator('.topbar-username')).toContainText('E2E Tester');
    await expect(page.getByRole('button', { name: 'Sign out' })).toBeVisible();
  });

  test('sign out redirects to login', async ({ page }) => {
    await page.route('**/api/auth/me', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ user: MOCK_USER }),
      })
    );
    await page.route('**/api/auth/logout', (route) =>
      route.fulfill({ status: 200, body: '{}' })
    );

    await page.goto('/app/dashboards');
    await expect(page.locator('.app-shell')).toBeVisible({ timeout: 10_000 });

    await page.getByRole('button', { name: 'Sign out' }).click();

    // Should redirect to login
    await expect(page).toHaveURL(/\/login/, { timeout: 10_000 });
    await expect(page.locator('.login-card')).toBeVisible();
  });

  test('login form validates required fields', async ({ page }) => {
    await page.route('**/api/auth/me', (route) =>
      route.fulfill({ status: 401, body: 'Unauthorized' })
    );

    await page.goto('/login');
    await expect(page.locator('.login-card')).toBeVisible({ timeout: 10_000 });

    // Email and password inputs are required
    const emailInput = page.locator('#login-email');
    const passwordInput = page.locator('#login-password');
    await expect(emailInput).toHaveAttribute('required', '');
    await expect(passwordInput).toHaveAttribute('required', '');
  });

  test('login page has no shell chrome', async ({ page }) => {
    await page.route('**/api/auth/me', (route) =>
      route.fulfill({ status: 401, body: 'Unauthorized' })
    );

    await page.goto('/login');
    await expect(page.locator('.login-card')).toBeVisible({ timeout: 10_000 });

    // Sidebar and topbar should NOT be visible on login page
    await expect(page.locator('.app-sidebar')).toBeHidden();
    await expect(page.locator('.topbar')).toBeHidden();
  });

  test('API 401 during session triggers redirect to login', async ({ page }) => {
    let sessionValid = true;

    await page.route('**/api/auth/me', (route) =>
      route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ user: MOCK_USER }),
      })
    );

    await page.goto('/app/dashboards');
    await expect(page.locator('.app-shell')).toBeVisible({ timeout: 10_000 });

    // Simulate session expiry: the onUnauthorized callback sets user to null
    // and navigates to /login. Trigger it by evaluating the auth context.
    // Since we can't directly call onUnauthorized from Playwright, simulate
    // by navigating away and back with a 401 on /api/auth/me.
    await page.route('**/api/auth/me', (route) =>
      route.fulfill({ status: 401, body: 'Session expired' })
    );

    // Reload triggers getMe() which will get 401
    await page.reload();
    await expect(page).toHaveURL(/\/login/, { timeout: 10_000 });
  });
});

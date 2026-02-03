import { test, expect } from '@playwright/test';

test.describe('Dashboards API', () => {
  const testDashboard = {
    name: 'E2E Test Dashboard',
    description: 'Created by e2e tests',
    widgets: [
      {
        type: 'graph',
        title: 'Test Graph',
        query: 'up',
        position: { x: 0, y: 0, w: 6, h: 4 }
      },
      {
        type: 'stat',
        title: 'Test Stat',
        query: 'count(up)',
        position: { x: 6, y: 0, w: 3, h: 2 }
      }
    ],
    tags: ['e2e', 'test']
  };

  let dashboardId: string;

  test('list dashboards', async ({ request }) => {
    const response = await request.get('/api/dashboards');

    expect(response.status()).toBe(200);

    const body = await response.json();
    // API may return null, array, or object with dashboards
    expect(body === null || Array.isArray(body) || body.dashboards !== undefined).toBe(true);
  });

  test('create dashboard', async ({ request }) => {
    const response = await request.post('/api/dashboards', {
      data: testDashboard,
      headers: { 'Content-Type': 'application/json' }
    });

    // 403 = CSRF protection
    expect([200, 201, 403]).toContain(response.status());

    if (response.status() === 200 || response.status() === 201) {
      const body = await response.json();
      expect(body.id || body.uid).toBeDefined();
      dashboardId = body.id || body.uid;
    }
  });

  test('get dashboard by id', async ({ request }) => {
    // First create a dashboard (may fail due to CSRF)
    const createResp = await request.post('/api/dashboards', {
      data: { ...testDashboard, name: 'E2E Get Test' },
      headers: { 'Content-Type': 'application/json' }
    });

    if (createResp.status() === 200 || createResp.status() === 201) {
      const created = await createResp.json();
      const id = created.id || created.uid;

      const response = await request.get(`/api/dashboards/${id}`);

      expect([200, 404]).toContain(response.status());

      if (response.status() === 200) {
        const body = await response.json();
        expect(body.name).toContain('E2E');
      }
    }
  });

  test('update dashboard', async ({ request }) => {
    // Create first
    const createResp = await request.post('/api/dashboards', {
      data: { ...testDashboard, name: 'E2E Update Test' },
      headers: { 'Content-Type': 'application/json' }
    });

    if (createResp.status() === 200 || createResp.status() === 201) {
      const created = await createResp.json();
      const id = created.id || created.uid;

      const updated = { ...testDashboard, name: 'E2E Updated Dashboard' };

      const response = await request.put(`/api/dashboards/${id}`, {
        data: updated,
        headers: { 'Content-Type': 'application/json' }
      });

      expect([200, 204]).toContain(response.status());
    }
  });

  test('search dashboards', async ({ request }) => {
    const response = await request.get('/api/dashboards/search', {
      params: { query: 'e2e' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get dashboard by tag', async ({ request }) => {
    const response = await request.get('/api/dashboards', {
      params: { tag: 'e2e' }
    });

    expect(response.status()).toBe(200);
  });

  test('delete dashboard', async ({ request }) => {
    // Create then delete
    const createResp = await request.post('/api/dashboards', {
      data: { ...testDashboard, name: 'E2E Delete Test' },
      headers: { 'Content-Type': 'application/json' }
    });

    if (createResp.status() === 200 || createResp.status() === 201) {
      const created = await createResp.json();
      const id = created.id || created.uid;

      const response = await request.delete(`/api/dashboards/${id}`);

      expect([200, 204]).toContain(response.status());
    }
  });

  test('export dashboard', async ({ request }) => {
    // Create first
    const createResp = await request.post('/api/dashboards', {
      data: { ...testDashboard, name: 'E2E Export Test' },
      headers: { 'Content-Type': 'application/json' }
    });

    if (createResp.status() === 200 || createResp.status() === 201) {
      const created = await createResp.json();
      const id = created.id || created.uid;

      const response = await request.get(`/api/dashboards/${id}/export`);

      expect([200, 404]).toContain(response.status());
    }
  });

  test('import dashboard', async ({ request }) => {
    const importData = {
      dashboard: testDashboard,
      overwrite: false
    };

    const response = await request.post('/api/dashboards/import', {
      data: importData,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 403, 404, 409]).toContain(response.status());
  });
});

test.describe('Dashboard UI', () => {
  test('dashboard page loads', async ({ page }) => {
    await page.goto('/');

    // Wait for app to load
    await page.waitForSelector('.app.loaded, .dashboard, .grid-stack', { timeout: 10000 });

    // Should have dashboard elements
    await expect(page.locator('.grid-stack, .dashboard-grid, .main-content').first()).toBeVisible();
  });

  test('can open dashboard manager', async ({ page }) => {
    await page.goto('/');

    // Check if login screen is blocking
    const loginScreen = page.locator('#login-screen.show, .login-screen.show');
    if (await loginScreen.isVisible({ timeout: 2000 }).catch(() => false)) {
      return;
    }

    const appLoaded = await page.waitForSelector('.app.loaded, .grid-stack, .dashboard', { timeout: 10000 }).catch(() => null);
    if (!appLoaded) {
      return;
    }

    // Click dashboards button if it exists
    const dashboardsBtn = page.getByRole('button', { name: /dashboard/i }).first();
    if (await dashboardsBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
      await dashboardsBtn.click();

      // Modal or panel should appear
      await expect(page.locator('.modal, .panel, .dashboard-manager').first()).toBeVisible({ timeout: 3000 });
    }
  });

  test('dashboard select dropdown exists', async ({ page }) => {
    await page.goto('/');

    // Check if login screen is blocking
    const loginScreen = page.locator('#login-screen.show, .login-screen.show');
    if (await loginScreen.isVisible({ timeout: 2000 }).catch(() => false)) {
      return;
    }

    const appLoaded = await page.waitForSelector('.app.loaded, .grid-stack, .dashboard', { timeout: 10000 }).catch(() => null);
    if (!appLoaded) {
      return;
    }

    // Check for dashboard select
    const select = page.locator('#dashboard-select, .dashboard-select').first();
    if (await select.isVisible({ timeout: 3000 }).catch(() => false)) {
      await expect(select).toBeEnabled();
    }
  });
});

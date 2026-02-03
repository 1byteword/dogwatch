import { test, expect } from '@playwright/test';

test.describe('Traces Page', () => {
  test('page loads', async ({ page }) => {
    await page.goto('/traces.html');

    // Check page loads
    await expect(page.locator('body')).toBeVisible();

    // Look for trace-related elements
    const hasTraceContent = await page.locator('.trace, [class*="trace"], h1').first().isVisible();
    expect(hasTraceContent).toBe(true);
  });

  test('traces API endpoint works', async ({ request }) => {
    const response = await request.get('/api/traces', {
      params: {
        limit: '10'
      }
    });

    // Should return 200
    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(Array.isArray(body) || body.traces !== undefined).toBe(true);
  });
});

test.describe('Logs Page', () => {
  test('page loads', async ({ page }) => {
    await page.goto('/logs.html');

    // Check page loads
    await expect(page.locator('body')).toBeVisible();

    // Look for log-related elements
    const hasLogContent = await page.locator('.log, [class*="log"], h1, .page-header').first().isVisible();
    expect(hasLogContent).toBe(true);
  });

  test('logs API endpoint works', async ({ request }) => {
    const response = await request.get('/api/logs', {
      params: {
        limit: '10'
      }
    });

    // Should return 200
    expect(response.status()).toBe(200);
  });

  test('logs ingest endpoint works', async ({ request }) => {
    const response = await request.post('/api/logs/ingest', {
      data: JSON.stringify({
        streams: [{
          stream: { source: 'e2e-test' },
          values: [[`${Date.now()}000000`, 'Test log message from e2e']]
        }]
      }),
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 204]).toContain(response.status());
  });
});

test.describe('Security Page', () => {
  test('page loads', async ({ page }) => {
    await page.goto('/security.html');

    await expect(page.locator('body')).toBeVisible();

    // Should have security-related content
    await expect(page.locator('h1, .page-header')).toContainText(/security/i);
  });
});

test.describe('Cost Page', () => {
  test('page loads', async ({ page }) => {
    await page.goto('/cost.html');

    await expect(page.locator('body')).toBeVisible();

    // Should have cost-related content
    await expect(page.locator('h1, .page-header, .cost')).toBeVisible();
  });
});

test.describe('BubbleUp Page', () => {
  test('page loads', async ({ page }) => {
    await page.goto('/bubbleup.html');

    await expect(page.locator('body')).toBeVisible();
  });
});

test.describe('Admin Page', () => {
  test('page loads', async ({ page }) => {
    await page.goto('/admin.html');

    await expect(page.locator('body')).toBeVisible();
  });
});

test.describe('Settings Page', () => {
  test('page loads', async ({ page }) => {
    await page.goto('/settings.html');

    await expect(page.locator('body')).toBeVisible();

    // Should have tabs or settings sections
    await expect(page.locator('.tab, .settings, h1')).toBeVisible();
  });
});

test.describe('Import/Migration Page', () => {
  test('page loads', async ({ page }) => {
    await page.goto('/import.html');

    await expect(page.locator('body')).toBeVisible();
  });
});

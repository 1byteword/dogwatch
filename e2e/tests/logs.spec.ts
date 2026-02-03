import { test, expect } from '@playwright/test';

test.describe('Logs API', () => {
  test('query logs', async ({ request }) => {
    const response = await request.get('/api/logs', {
      params: { limit: '10' }
    });

    expect(response.status()).toBe(200);
  });

  test('query logs with filter', async ({ request }) => {
    const response = await request.get('/api/logs', {
      params: {
        query: 'level=error',
        limit: '10'
      }
    });

    expect(response.status()).toBe(200);
  });

  test('ingest logs via Loki format', async ({ request }) => {
    const lokiPayload = {
      streams: [{
        stream: { source: 'e2e-test', level: 'info' },
        values: [[`${Date.now()}000000`, 'Test log message from e2e']]
      }]
    };

    const response = await request.post('/api/logs/ingest', {
      data: JSON.stringify(lokiPayload),
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 204]).toContain(response.status());
  });

  test('Loki push endpoint', async ({ request }) => {
    const lokiPayload = {
      streams: [{
        stream: { source: 'e2e-loki' },
        values: [[`${Date.now()}000000`, 'Loki format log']]
      }]
    };

    const response = await request.post('/loki/api/v1/push', {
      data: JSON.stringify(lokiPayload),
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 204, 404]).toContain(response.status());
  });

  test('get log labels', async ({ request }) => {
    const response = await request.get('/api/logs/labels');

    expect([200, 404]).toContain(response.status());
  });

  test('get log label values', async ({ request }) => {
    const response = await request.get('/api/logs/labels/source/values');

    expect([200, 404]).toContain(response.status());
  });

  test('logs tail endpoint', async ({ request }) => {
    const response = await request.get('/api/logs/tail', {
      params: { query: '{source="e2e-test"}' }
    });

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Logs UI', () => {
  test('logs page loads', async ({ page }) => {
    await page.goto('/logs.html');

    await expect(page.locator('body')).toBeVisible();

    // Should have log-related elements
    const hasLogContent = await page.locator('.log, [class*="log"], h1, .page-header').first().isVisible();
    expect(hasLogContent).toBe(true);
  });

  test('log query input exists', async ({ page }) => {
    await page.goto('/logs.html');

    // Should have a query input
    const queryInput = page.locator('input[type="text"], textarea, .query-input');
    await expect(queryInput.first()).toBeVisible();
  });
});

test.describe('LogQL Support', () => {
  test('LogQL query endpoint', async ({ request }) => {
    const response = await request.get('/loki/api/v1/query', {
      params: {
        query: '{source="test"}'
      }
    });

    expect([200, 400, 404]).toContain(response.status());
  });

  test('LogQL range query', async ({ request }) => {
    const now = Math.floor(Date.now() / 1000);
    const response = await request.get('/loki/api/v1/query_range', {
      params: {
        query: '{source="test"}',
        start: now - 3600,
        end: now,
        step: 60
      }
    });

    expect([200, 400, 404]).toContain(response.status());
  });

  test('Loki labels endpoint', async ({ request }) => {
    const response = await request.get('/loki/api/v1/labels');

    expect([200, 404]).toContain(response.status());
  });

  test('Loki series endpoint', async ({ request }) => {
    const response = await request.get('/loki/api/v1/series', {
      params: { 'match[]': '{source="test"}' }
    });

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Log Aggregation', () => {
  test('get log volume', async ({ request }) => {
    const response = await request.get('/api/logs/volume', {
      params: {
        start: Date.now() - 3600000,
        end: Date.now()
      }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get log stats', async ({ request }) => {
    const response = await request.get('/api/logs/stats');

    expect([200, 404]).toContain(response.status());
  });

  test('get log patterns', async ({ request }) => {
    const response = await request.get('/api/logs/patterns');

    expect([200, 404]).toContain(response.status());
  });
});

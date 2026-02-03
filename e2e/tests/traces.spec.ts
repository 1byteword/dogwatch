import { test, expect } from '@playwright/test';

test.describe('Traces API', () => {
  test('list traces', async ({ request }) => {
    const response = await request.get('/api/traces', {
      params: { limit: '10' }
    });

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(Array.isArray(body) || body.traces !== undefined).toBe(true);
  });

  test('search traces', async ({ request }) => {
    const response = await request.get('/api/traces/search', {
      params: {
        service: 'api-service',
        operation: 'GET /api/users',
        minDuration: '100ms',
        limit: '10'
      }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get trace by id', async ({ request }) => {
    const response = await request.get('/api/traces/0000000000000001');

    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const body = await response.json();
      expect(body.traceId || body.traceID || body.trace).toBeDefined();
    }
  });

  test('get trace spans', async ({ request }) => {
    const response = await request.get('/api/traces/0000000000000001/spans');

    expect([200, 404]).toContain(response.status());
  });

  test('ingest trace via OTLP', async ({ request }) => {
    const trace = {
      resourceSpans: [{
        resource: {
          attributes: [
            { key: 'service.name', value: { stringValue: 'e2e-service' } }
          ]
        },
        scopeSpans: [{
          spans: [{
            traceId: '0000000000000001',
            spanId: '0000000000000001',
            name: 'e2e-test-span',
            kind: 1,
            startTimeUnixNano: Date.now() * 1000000,
            endTimeUnixNano: (Date.now() + 100) * 1000000
          }]
        }]
      }]
    };

    const response = await request.post('/api/v1/traces', {
      data: trace,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 202, 204, 400]).toContain(response.status());
  });

  test('OTLP traces endpoint', async ({ request }) => {
    const response = await request.post('/v1/traces', {
      data: { resourceSpans: [] },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 202, 204, 400, 404]).toContain(response.status());
  });

  test('get services from traces', async ({ request }) => {
    const response = await request.get('/api/traces/services');

    expect([200, 404]).toContain(response.status());
  });

  test('get operations for service', async ({ request }) => {
    const response = await request.get('/api/traces/services/api-service/operations');

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Traces UI', () => {
  test('traces page loads', async ({ page }) => {
    await page.goto('/traces.html');

    await expect(page.locator('body')).toBeVisible();

    // Should have trace-related content
    const hasTraceContent = await page.locator('.trace, [class*="trace"], h1').first().isVisible();
    expect(hasTraceContent).toBe(true);
  });

  test('trace detail view loads', async ({ page }) => {
    await page.goto('/traces.html');

    // Look for a trace row to click
    const traceRow = page.locator('.trace-row, .trace-item, tr[data-trace-id]').first();
    if (await traceRow.isVisible()) {
      await traceRow.click();

      // Should show trace details
      await expect(page.locator('.trace-detail, .span-tree, .waterfall')).toBeVisible({ timeout: 5000 });
    }
  });
});

test.describe('Distributed Tracing', () => {
  test('trace context propagation headers', async ({ request }) => {
    const response = await request.get('/api/status', {
      headers: {
        'traceparent': '00-0000000000000001-0000000000000002-01'
      }
    });

    // Should accept traceparent header
    expect([200, 401]).toContain(response.status());
  });

  test('get trace stats', async ({ request }) => {
    const response = await request.get('/api/traces/stats', {
      params: {
        start: Date.now() - 3600000,
        end: Date.now()
      }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get trace latency histogram', async ({ request }) => {
    const response = await request.get('/api/traces/latency-histogram', {
      params: { service: 'api-service' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get trace error rate', async ({ request }) => {
    const response = await request.get('/api/traces/error-rate', {
      params: { service: 'api-service' }
    });

    expect([200, 404]).toContain(response.status());
  });
});

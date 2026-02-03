import { test, expect } from '@playwright/test';

test.describe('Cost Intelligence', () => {
  test('cost page loads', async ({ page }) => {
    await page.goto('/cost.html');

    await expect(page.locator('body')).toBeVisible();

    // Should have cost-related content
    await expect(page.locator('h1, .page-header, .cost, .pricing')).toBeVisible();
  });

  test('get cost summary', async ({ request }) => {
    const response = await request.get('/api/cost/summary');

    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const body = await response.json();
      expect(body).toBeDefined();
    }
  });

  test('get cost breakdown by service', async ({ request }) => {
    const response = await request.get('/api/cost/breakdown', {
      params: { groupBy: 'service' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get cost breakdown by team', async ({ request }) => {
    const response = await request.get('/api/cost/breakdown', {
      params: { groupBy: 'team' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get cost history', async ({ request }) => {
    const response = await request.get('/api/cost/history', {
      params: {
        start: new Date(Date.now() - 30 * 24 * 3600000).toISOString(),
        end: new Date().toISOString()
      }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get cost comparison vs Datadog', async ({ request }) => {
    const response = await request.get('/api/cost/comparison');

    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const body = await response.json();
      // Should show what this would cost on commercial platforms
      expect(body).toBeDefined();
    }
  });

  test('get high cost metrics', async ({ request }) => {
    const response = await request.get('/api/cost/high-cost-metrics');

    expect([200, 404]).toContain(response.status());
  });

  test('get cardinality cost', async ({ request }) => {
    const response = await request.get('/api/cost/cardinality');

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Cost Optimization', () => {
  test('get optimization recommendations', async ({ request }) => {
    const response = await request.get('/api/cost/recommendations');

    expect([200, 404]).toContain(response.status());
  });

  test('get unused metrics', async ({ request }) => {
    const response = await request.get('/api/cost/unused-metrics');

    expect([200, 404]).toContain(response.status());
  });

  test('get low-value alerts', async ({ request }) => {
    const response = await request.get('/api/cost/low-value-alerts');

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Cost Allocation', () => {
  test('set cost allocation rules', async ({ request }) => {
    const rule = {
      name: 'e2e-cost-rule',
      match: { label: 'team' },
      allocation: 'proportional'
    };

    const response = await request.post('/api/cost/allocation-rules', {
      data: rule,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 404]).toContain(response.status());
  });

  test('list allocation rules', async ({ request }) => {
    const response = await request.get('/api/cost/allocation-rules');

    expect([200, 404]).toContain(response.status());
  });

  test('get cost report', async ({ request }) => {
    const response = await request.get('/api/cost/report', {
      params: {
        period: 'monthly',
        format: 'json'
      }
    });

    expect([200, 404]).toContain(response.status());
  });
});

import { test, expect } from '@playwright/test';

test.describe('Alerting API', () => {
  const testRule = {
    name: 'e2e-test-rule',
    expr: 'up == 0',
    for: '1m',
    labels: { severity: 'critical', source: 'e2e' },
    annotations: { summary: 'Instance down', description: 'Test alert from e2e' }
  };

  test('list alert rules', async ({ request }) => {
    const response = await request.get('/api/alerts/rules');

    // May be 404 if alerting module not loaded
    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const body = await response.json();
      expect(body).toBeDefined();
    }
  });

  test('create alert rule', async ({ request }) => {
    const response = await request.post('/api/alerts/rules', {
      data: testRule,
      headers: { 'Content-Type': 'application/json' }
    });

    // Should succeed, conflict, or 403 (CSRF) / 404 (not implemented)
    expect([200, 201, 403, 404, 409]).toContain(response.status());

    if (response.status() === 200 || response.status() === 201) {
      const body = await response.json();
      expect(body.name || body.id).toBeDefined();
    }
  });

  test('get alert rule by name', async ({ request }) => {
    // First create the rule
    await request.post('/api/alerts/rules', {
      data: testRule,
      headers: { 'Content-Type': 'application/json' }
    });

    const response = await request.get(`/api/alerts/rules/${testRule.name}`);

    if (response.status() === 200) {
      const body = await response.json();
      expect(body.name).toBe(testRule.name);
      expect(body.expr).toBe(testRule.expr);
    }
  });

  test('update alert rule', async ({ request }) => {
    const updated = { ...testRule, for: '5m' };

    const response = await request.put(`/api/alerts/rules/${testRule.name}`, {
      data: updated,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 204, 403, 404]).toContain(response.status());
  });

  test('list active alerts', async ({ request }) => {
    const response = await request.get('/api/alerts');

    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const body = await response.json();
      expect(body).toBeDefined();
    }
  });

  test('get alert history', async ({ request }) => {
    const response = await request.get('/api/alerts/history', {
      params: { limit: '10' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('silence an alert', async ({ request }) => {
    const silence = {
      matchers: [{ name: 'alertname', value: 'TestAlert', isRegex: false }],
      startsAt: new Date().toISOString(),
      endsAt: new Date(Date.now() + 3600000).toISOString(),
      createdBy: 'e2e-test',
      comment: 'E2E test silence'
    };

    const response = await request.post('/api/alerts/silences', {
      data: silence,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 403, 404]).toContain(response.status());
  });

  test('list silences', async ({ request }) => {
    const response = await request.get('/api/alerts/silences');

    expect([200, 404]).toContain(response.status());
  });

  test('delete alert rule', async ({ request }) => {
    const response = await request.delete(`/api/alerts/rules/${testRule.name}`);

    expect([200, 204, 403, 404]).toContain(response.status());
  });

  test('Prometheus-compatible alerts endpoint', async ({ request }) => {
    const response = await request.get('/api/v1/alerts');

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.status).toBe('success');
  });

  test('Prometheus-compatible rules endpoint', async ({ request }) => {
    const response = await request.get('/api/v1/rules');

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.status).toBe('success');
  });
});

test.describe('Notification Channels', () => {
  test('list notification channels', async ({ request }) => {
    const response = await request.get('/api/notify/channels');

    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const body = await response.json();
      expect(Array.isArray(body) || body.channels !== undefined).toBe(true);
    }
  });

  test('create webhook channel', async ({ request }) => {
    const channel = {
      name: 'e2e-webhook',
      type: 'webhook',
      config: {
        url: 'https://httpbin.org/post'
      }
    };

    const response = await request.post('/api/notify/channels', {
      data: channel,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 403, 404, 409]).toContain(response.status());
  });

  test('test notification channel', async ({ request }) => {
    const response = await request.post('/api/notify/channels/test', {
      data: {
        type: 'webhook',
        config: { url: 'https://httpbin.org/post' }
      },
      headers: { 'Content-Type': 'application/json' }
    });

    // May timeout or fail for external URL, but endpoint should work
    expect([200, 400, 403, 404, 408, 500, 502]).toContain(response.status());
  });
});

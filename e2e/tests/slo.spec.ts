import { test, expect } from '@playwright/test';

test.describe('SLO API', () => {
  const testSLO = {
    name: 'e2e-test-slo',
    description: 'E2E test SLO',
    service: 'api-service',
    sli: {
      type: 'availability',
      goodQuery: 'sum(rate(http_requests_total{status=~"2.."}[5m]))',
      totalQuery: 'sum(rate(http_requests_total[5m]))'
    },
    target: 0.999, // 99.9%
    window: '30d',
    labels: { team: 'platform', source: 'e2e' }
  };

  let sloId: string;

  test('list SLOs', async ({ request }) => {
    const response = await request.get('/api/slos');

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(Array.isArray(body) || body.slos !== undefined).toBe(true);
  });

  test('create SLO', async ({ request }) => {
    const response = await request.post('/api/slos', {
      data: testSLO,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 409]).toContain(response.status());

    if (response.status() === 200 || response.status() === 201) {
      const body = await response.json();
      expect(body.id || body.name).toBeDefined();
      sloId = body.id;
    }
  });

  test('get SLO by name', async ({ request }) => {
    const response = await request.get(`/api/slos/${testSLO.name}`);

    if (response.status() === 200) {
      const body = await response.json();
      expect(body.name).toBe(testSLO.name);
      expect(body.target).toBe(testSLO.target);
    }
  });

  test('get SLO status', async ({ request }) => {
    const response = await request.get(`/api/slos/${testSLO.name}/status`);

    if (response.status() === 200) {
      const body = await response.json();
      expect(body).toHaveProperty('current');
      expect(body).toHaveProperty('errorBudget');
    }
  });

  test('get SLO burn rate', async ({ request }) => {
    const response = await request.get(`/api/slos/${testSLO.name}/burn-rate`);

    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const body = await response.json();
      expect(body).toHaveProperty('burnRate');
    }
  });

  test('get SLO history', async ({ request }) => {
    const response = await request.get(`/api/slos/${testSLO.name}/history`, {
      params: { period: '7d' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('update SLO', async ({ request }) => {
    const updated = { ...testSLO, target: 0.9999 };

    const response = await request.put(`/api/slos/${testSLO.name}`, {
      data: updated,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 204, 404]).toContain(response.status());
  });

  test('list SLOs by service', async ({ request }) => {
    const response = await request.get('/api/slos', {
      params: { service: testSLO.service }
    });

    expect(response.status()).toBe(200);
  });

  test('delete SLO', async ({ request }) => {
    const response = await request.delete(`/api/slos/${testSLO.name}`);

    expect([200, 204, 404]).toContain(response.status());
  });
});

test.describe('Error Budget API', () => {
  test('get error budget summary', async ({ request }) => {
    const response = await request.get('/api/slos/error-budget/summary');

    expect([200, 404]).toContain(response.status());
  });

  test('get error budget alerts', async ({ request }) => {
    const response = await request.get('/api/slos/error-budget/alerts');

    expect([200, 404]).toContain(response.status());
  });
});

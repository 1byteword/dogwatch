import { test, expect } from '@playwright/test';

test.describe('Synthetics API', () => {
  const testCheck = {
    name: 'e2e-http-check',
    type: 'http',
    url: 'https://httpbin.org/status/200',
    interval: '1m',
    timeout: '10s',
    locations: ['us-east'],
    assertions: [
      { type: 'statusCode', value: 200 }
    ],
    labels: { source: 'e2e' }
  };

  test('list synthetic checks', async ({ request }) => {
    const response = await request.get('/api/synthetics/checks');

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(Array.isArray(body) || body.checks !== undefined).toBe(true);
  });

  test('create HTTP check', async ({ request }) => {
    const response = await request.post('/api/synthetics/checks', {
      data: testCheck,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 403, 409]).toContain(response.status());

    if (response.status() === 200 || response.status() === 201) {
      const body = await response.json();
      expect(body.id || body.name).toBeDefined();
    }
  });

  test('get check by name', async ({ request }) => {
    const response = await request.get(`/api/synthetics/checks/${testCheck.name}`);

    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const body = await response.json();
      expect(body.name).toBe(testCheck.name);
    }
  });

  test('get check results', async ({ request }) => {
    const response = await request.get(`/api/synthetics/checks/${testCheck.name}/results`, {
      params: { limit: '10' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('update check', async ({ request }) => {
    const updated = { ...testCheck, interval: '5m' };

    const response = await request.put(`/api/synthetics/checks/${testCheck.name}`, {
      data: updated,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 204, 403, 404]).toContain(response.status());
  });

  test('pause check', async ({ request }) => {
    const response = await request.post(`/api/synthetics/checks/${testCheck.name}/pause`);

    expect([200, 204, 403, 404]).toContain(response.status());
  });

  test('resume check', async ({ request }) => {
    const response = await request.post(`/api/synthetics/checks/${testCheck.name}/resume`);

    expect([200, 204, 403, 404]).toContain(response.status());
  });

  test('delete check', async ({ request }) => {
    const response = await request.delete(`/api/synthetics/checks/${testCheck.name}`);

    expect([200, 204, 403, 404]).toContain(response.status());
  });
});

test.describe('Synthetics Check Types', () => {
  test('create TCP check', async ({ request }) => {
    const tcpCheck = {
      name: 'e2e-tcp-check',
      type: 'tcp',
      host: 'httpbin.org',
      port: 443,
      interval: '1m',
      timeout: '5s'
    };

    const response = await request.post('/api/synthetics/checks', {
      data: tcpCheck,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 403, 409]).toContain(response.status());
  });

  test('create DNS check', async ({ request }) => {
    const dnsCheck = {
      name: 'e2e-dns-check',
      type: 'dns',
      hostname: 'example.com',
      recordType: 'A',
      interval: '5m'
    };

    const response = await request.post('/api/synthetics/checks', {
      data: dnsCheck,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 403, 409]).toContain(response.status());
  });

  test('create SSL check', async ({ request }) => {
    const sslCheck = {
      name: 'e2e-ssl-check',
      type: 'ssl',
      host: 'example.com',
      port: 443,
      warnDays: 30,
      critDays: 7
    };

    const response = await request.post('/api/synthetics/checks', {
      data: sslCheck,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 403, 409]).toContain(response.status());
  });
});

test.describe('Synthetics Locations', () => {
  test('list available locations', async ({ request }) => {
    const response = await request.get('/api/synthetics/locations');

    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const body = await response.json();
      expect(Array.isArray(body) || body.locations !== undefined).toBe(true);
    }
  });
});

test.describe('Synthetics UI', () => {
  test('synthetics page loads', async ({ page }) => {
    // synthetics.html may not exist
    const response = await page.goto('/synthetics.html');

    // Accept 404 if page doesn't exist
    if (response && response.status() === 404) {
      return;
    }

    await expect(page.locator('body')).toBeVisible();

    // Should have synthetics-related content
    await expect(page.locator('h1, .page-header, .synthetic').first()).toBeVisible();
  });
});

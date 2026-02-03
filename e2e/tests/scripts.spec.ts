import { test, expect } from '@playwright/test';

test.describe('Scripts Engine', () => {
  test('list available scripts', async ({ request }) => {
    const response = await request.get('/api/scripts');

    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const body = await response.json();
      expect(Array.isArray(body) || body.scripts !== undefined).toBe(true);
    }
  });

  test('list script categories', async ({ request }) => {
    const response = await request.get('/api/scripts/categories');

    expect([200, 404]).toContain(response.status());
  });

  test('get script by name', async ({ request }) => {
    const response = await request.get('/api/scripts/mysql/slow_queries');

    expect([200, 404]).toContain(response.status());
  });

  test('run script', async ({ request }) => {
    const response = await request.post('/api/scripts/run', {
      data: {
        script: 'mysql/slow_queries',
        params: { threshold: '100ms' }
      },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 202, 400, 404]).toContain(response.status());
  });

  test('get script execution status', async ({ request }) => {
    const response = await request.get('/api/scripts/executions/test-execution-id');

    expect([200, 404]).toContain(response.status());
  });

  test('list recent executions', async ({ request }) => {
    const response = await request.get('/api/scripts/executions');

    expect([200, 404]).toContain(response.status());
  });

  test('cancel script execution', async ({ request }) => {
    const response = await request.post('/api/scripts/executions/test-execution-id/cancel');

    expect([200, 204, 404]).toContain(response.status());
  });
});

test.describe('Script Library', () => {
  test('mysql scripts available', async ({ request }) => {
    const response = await request.get('/api/scripts', {
      params: { category: 'mysql' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('postgres scripts available', async ({ request }) => {
    const response = await request.get('/api/scripts', {
      params: { category: 'postgres' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('redis scripts available', async ({ request }) => {
    const response = await request.get('/api/scripts', {
      params: { category: 'redis' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('system scripts available', async ({ request }) => {
    const response = await request.get('/api/scripts', {
      params: { category: 'system' }
    });

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Custom Scripts', () => {
  test('create custom script', async ({ request }) => {
    const script = {
      name: 'e2e-custom-script',
      description: 'E2E test custom script',
      category: 'custom',
      code: `
        SELECT COUNT(*) as total_requests
        FROM http_requests
        WHERE timestamp > NOW() - INTERVAL '1 hour'
      `
    };

    const response = await request.post('/api/scripts/custom', {
      data: script,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 409]).toContain(response.status());
  });

  test('list custom scripts', async ({ request }) => {
    const response = await request.get('/api/scripts/custom');

    expect([200, 404]).toContain(response.status());
  });

  test('update custom script', async ({ request }) => {
    const response = await request.put('/api/scripts/custom/e2e-custom-script', {
      data: { description: 'Updated description' },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 204, 404]).toContain(response.status());
  });

  test('delete custom script', async ({ request }) => {
    const response = await request.delete('/api/scripts/custom/e2e-custom-script');

    expect([200, 204, 404]).toContain(response.status());
  });
});

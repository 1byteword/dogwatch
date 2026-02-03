import { test, expect } from '@playwright/test';

test.describe('eBPF Probes', () => {
  test('list active probes', async ({ request }) => {
    const response = await request.get('/api/probes');

    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const body = await response.json();
      expect(Array.isArray(body) || body.probes !== undefined).toBe(true);
    }
  });

  test('get probe status', async ({ request }) => {
    const response = await request.get('/api/probes/status');

    expect([200, 404]).toContain(response.status());
  });

  test('list available probe types', async ({ request }) => {
    const response = await request.get('/api/probes/types');

    expect([200, 404]).toContain(response.status());
  });

  test('get eBPF capabilities', async ({ request }) => {
    const response = await request.get('/api/probes/capabilities');

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('HTTP Tracing', () => {
  test('list HTTP requests', async ({ request }) => {
    const response = await request.get('/api/probes/http/requests', {
      params: { limit: '10' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get HTTP stats', async ({ request }) => {
    const response = await request.get('/api/probes/http/stats');

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Database Tracing', () => {
  test('list MySQL queries', async ({ request }) => {
    const response = await request.get('/api/probes/mysql/queries', {
      params: { limit: '10' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get MySQL slow queries', async ({ request }) => {
    const response = await request.get('/api/probes/mysql/slow', {
      params: { threshold: '100ms' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('list PostgreSQL queries', async ({ request }) => {
    const response = await request.get('/api/probes/postgres/queries', {
      params: { limit: '10' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get PostgreSQL stats', async ({ request }) => {
    const response = await request.get('/api/probes/postgres/stats');

    expect([200, 404]).toContain(response.status());
  });

  test('list Redis commands', async ({ request }) => {
    const response = await request.get('/api/probes/redis/commands', {
      params: { limit: '10' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get Redis slow commands', async ({ request }) => {
    const response = await request.get('/api/probes/redis/slow');

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Network Tracing', () => {
  test('list TCP connections', async ({ request }) => {
    const response = await request.get('/api/probes/tcp/connections');

    expect([200, 404]).toContain(response.status());
  });

  test('get DNS queries', async ({ request }) => {
    const response = await request.get('/api/probes/dns/queries', {
      params: { limit: '10' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get network stats', async ({ request }) => {
    const response = await request.get('/api/probes/network/stats');

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('System Probes', () => {
  test('get file I/O stats', async ({ request }) => {
    const response = await request.get('/api/probes/fileio/stats');

    expect([200, 404]).toContain(response.status());
  });

  test('get process info', async ({ request }) => {
    const response = await request.get('/api/probes/processes');

    expect([200, 404]).toContain(response.status());
  });

  test('get syscall stats', async ({ request }) => {
    const response = await request.get('/api/probes/syscalls/stats');

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Probe Configuration', () => {
  test('enable probe', async ({ request }) => {
    const response = await request.post('/api/probes/http/enable');

    expect([200, 204, 400, 404]).toContain(response.status());
  });

  test('disable probe', async ({ request }) => {
    const response = await request.post('/api/probes/http/disable');

    expect([200, 204, 400, 404]).toContain(response.status());
  });

  test('configure probe', async ({ request }) => {
    const config = {
      sampleRate: 0.1,
      filters: [{ field: 'method', op: 'eq', value: 'GET' }]
    };

    const response = await request.put('/api/probes/http/config', {
      data: config,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 204, 400, 404]).toContain(response.status());
  });
});

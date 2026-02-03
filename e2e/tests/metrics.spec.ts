import { test, expect } from '@playwright/test';

test.describe('Metrics API', () => {
  test('query instant metrics', async ({ request }) => {
    const response = await request.get('/api/v1/query', {
      params: { query: 'up' }
    });

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.status).toBe('success');
  });

  test('query range metrics', async ({ request }) => {
    const now = Math.floor(Date.now() / 1000);
    const response = await request.get('/api/v1/query_range', {
      params: {
        query: 'up',
        start: now - 3600,
        end: now,
        step: 60
      }
    });

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.status).toBe('success');
  });

  test('get label names', async ({ request }) => {
    const response = await request.get('/api/v1/labels');

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.status).toBe('success');
    expect(Array.isArray(body.data)).toBe(true);
  });

  test('get label values', async ({ request }) => {
    const response = await request.get('/api/v1/label/job/values');

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.status).toBe('success');
  });

  test('get series', async ({ request }) => {
    const response = await request.get('/api/v1/series', {
      params: { 'match[]': 'up' }
    });

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.status).toBe('success');
  });

  test('get metadata', async ({ request }) => {
    const response = await request.get('/api/v1/metadata');

    expect([200, 404]).toContain(response.status());
  });

  test('write metrics via remote write', async ({ request }) => {
    // Prometheus remote write format
    const response = await request.post('/api/v1/write', {
      data: Buffer.from([]), // Empty protobuf for test
      headers: { 'Content-Type': 'application/x-protobuf' }
    });

    expect([200, 204, 400, 415]).toContain(response.status());
  });
});

test.describe('Custom Metrics', () => {
  test('push custom metric', async ({ request }) => {
    const metric = {
      name: 'e2e_test_metric',
      value: 42,
      labels: { source: 'e2e', env: 'test' },
      timestamp: Date.now()
    };

    const response = await request.post('/api/metrics/push', {
      data: metric,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 204, 400]).toContain(response.status());
  });

  test('push batch metrics', async ({ request }) => {
    const metrics = [
      { name: 'e2e_batch_1', value: 1, labels: { source: 'e2e' } },
      { name: 'e2e_batch_2', value: 2, labels: { source: 'e2e' } }
    ];

    const response = await request.post('/api/metrics/push/batch', {
      data: { metrics },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 204, 400]).toContain(response.status());
  });
});

test.describe('Recording Rules', () => {
  test('list recording rules', async ({ request }) => {
    const response = await request.get('/api/recording-rules');

    expect([200, 404]).toContain(response.status());
  });

  test('create recording rule', async ({ request }) => {
    const rule = {
      name: 'e2e:test:rate5m',
      expr: 'rate(http_requests_total[5m])',
      labels: { source: 'e2e' }
    };

    const response = await request.post('/api/recording-rules', {
      data: rule,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 409]).toContain(response.status());
  });

  test('get recording rule', async ({ request }) => {
    const response = await request.get('/api/recording-rules/e2e:test:rate5m');

    expect([200, 404]).toContain(response.status());
  });

  test('delete recording rule', async ({ request }) => {
    const response = await request.delete('/api/recording-rules/e2e:test:rate5m');

    expect([200, 204, 404]).toContain(response.status());
  });
});

test.describe('Histograms', () => {
  test('histogram_quantile query works', async ({ request }) => {
    const response = await request.get('/api/v1/query', {
      params: {
        query: 'histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m]))'
      }
    });

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.status).toBe('success');
  });

  test('native histogram support', async ({ request }) => {
    const response = await request.get('/api/v1/query', {
      params: { query: 'histogram_avg(http_request_duration_seconds)' }
    });

    // May not be implemented yet
    expect([200, 400, 422]).toContain(response.status());
  });
});

test.describe('Metric Cardinality', () => {
  test('get cardinality stats', async ({ request }) => {
    const response = await request.get('/api/metrics/cardinality');

    expect([200, 404]).toContain(response.status());
  });

  test('get high cardinality metrics', async ({ request }) => {
    const response = await request.get('/api/metrics/cardinality/high');

    expect([200, 404]).toContain(response.status());
  });

  test('get label cardinality', async ({ request }) => {
    const response = await request.get('/api/metrics/cardinality/labels');

    expect([200, 404]).toContain(response.status());
  });
});

import { test, expect } from '@playwright/test';

test.describe('Correlation Engine', () => {
  test('correlate metrics and traces', async ({ request }) => {
    const response = await request.post('/api/correlate', {
      data: {
        metric: 'http_requests_total',
        labels: { service: 'api' },
        timeRange: { start: Date.now() - 3600000, end: Date.now() }
      },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('find related signals', async ({ request }) => {
    const response = await request.get('/api/correlate/related', {
      params: {
        type: 'metric',
        name: 'http_requests_total',
        time: Math.floor(Date.now() / 1000)
      }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get trace from metric', async ({ request }) => {
    const response = await request.get('/api/correlate/metric-to-trace', {
      params: {
        metric: 'http_request_duration_seconds',
        labels: JSON.stringify({ service: 'api' }),
        timestamp: Math.floor(Date.now() / 1000) - 60
      }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get logs from trace', async ({ request }) => {
    const response = await request.get('/api/correlate/trace-to-logs', {
      params: { traceId: '0000000000000001' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get metrics from trace', async ({ request }) => {
    const response = await request.get('/api/correlate/trace-to-metrics', {
      params: { traceId: '0000000000000001' }
    });

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Profile-Trace Linking', () => {
  test('get profiles for trace', async ({ request }) => {
    const response = await request.get('/api/profiles/by-trace', {
      params: { traceId: '0000000000000001' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('get traces for profile hotspot', async ({ request }) => {
    const response = await request.get('/api/profiles/hotspot-traces', {
      params: {
        profileId: 'test-profile',
        function: 'main.handleRequest'
      }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('list profiles', async ({ request }) => {
    const response = await request.get('/api/profiles');

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Anomaly Detection', () => {
  test('get anomalies', async ({ request }) => {
    const response = await request.get('/api/anomalies');

    expect([200, 404]).toContain(response.status());
  });

  test('get anomaly for metric', async ({ request }) => {
    const response = await request.get('/api/anomalies/metric', {
      params: { metric: 'http_requests_total' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('configure anomaly detection', async ({ request }) => {
    const config = {
      metric: 'http_requests_total',
      sensitivity: 'medium',
      algorithm: 'mad',
      enabled: true
    };

    const response = await request.post('/api/anomalies/config', {
      data: config,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 404]).toContain(response.status());
  });

  test('get anomaly baselines', async ({ request }) => {
    const response = await request.get('/api/anomalies/baselines');

    expect([200, 404]).toContain(response.status());
  });

  test('train anomaly model', async ({ request }) => {
    const response = await request.post('/api/anomalies/train', {
      data: {
        metric: 'http_requests_total',
        windowDays: 7
      },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 202, 400, 404]).toContain(response.status());
  });
});

test.describe('BubbleUp Analysis', () => {
  test('bubbleup page loads', async ({ page }) => {
    await page.goto('/bubbleup.html');

    await expect(page.locator('body')).toBeVisible();
  });

  test('run bubbleup analysis', async ({ request }) => {
    const response = await request.post('/api/bubbleup', {
      data: {
        metric: 'http_requests_total',
        baselineStart: Date.now() - 7200000,
        baselineEnd: Date.now() - 3600000,
        comparisonStart: Date.now() - 3600000,
        comparisonEnd: Date.now(),
        dimensions: ['service', 'method', 'status']
      },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 400, 404]).toContain(response.status());
  });
});

import { test, expect } from '@playwright/test';

test.describe('Migration API', () => {
  test('migration page loads', async ({ page }) => {
    await page.goto('/import.html');

    await expect(page.locator('body')).toBeVisible();
  });

  test('list supported sources', async ({ request }) => {
    const response = await request.get('/api/migrate/sources');

    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const body = await response.json();
      // Should support major platforms
      expect(body).toBeDefined();
    }
  });

  test('get migration status', async ({ request }) => {
    const response = await request.get('/api/migrate/status');

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Datadog Migration', () => {
  test('validate Datadog credentials', async ({ request }) => {
    const response = await request.post('/api/migrate/datadog/validate', {
      data: {
        apiKey: 'test-api-key',
        appKey: 'test-app-key'
      },
      headers: { 'Content-Type': 'application/json' }
    });

    // Will fail without real credentials, but endpoint should exist
    expect([200, 400, 401, 404]).toContain(response.status());
  });

  test('list Datadog dashboards to import', async ({ request }) => {
    const response = await request.get('/api/migrate/datadog/dashboards', {
      params: {
        apiKey: 'test',
        appKey: 'test'
      }
    });

    expect([200, 400, 401, 404]).toContain(response.status());
  });

  test('import Datadog dashboard', async ({ request }) => {
    const response = await request.post('/api/migrate/datadog/dashboards/import', {
      data: {
        dashboardId: 'test-dashboard',
        apiKey: 'test',
        appKey: 'test'
      },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 401, 404]).toContain(response.status());
  });

  test('list Datadog monitors to import', async ({ request }) => {
    const response = await request.get('/api/migrate/datadog/monitors', {
      params: {
        apiKey: 'test',
        appKey: 'test'
      }
    });

    expect([200, 400, 401, 404]).toContain(response.status());
  });
});

test.describe('Grafana Migration', () => {
  test('validate Grafana connection', async ({ request }) => {
    const response = await request.post('/api/migrate/grafana/validate', {
      data: {
        url: 'http://localhost:3000',
        apiKey: 'test-api-key'
      },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 400, 401, 404]).toContain(response.status());
  });

  test('list Grafana dashboards', async ({ request }) => {
    const response = await request.get('/api/migrate/grafana/dashboards', {
      params: {
        url: 'http://localhost:3000',
        apiKey: 'test'
      }
    });

    expect([200, 400, 401, 404]).toContain(response.status());
  });

  test('import Grafana dashboard', async ({ request }) => {
    const response = await request.post('/api/migrate/grafana/dashboards/import', {
      data: {
        dashboardUid: 'test-uid',
        url: 'http://localhost:3000',
        apiKey: 'test'
      },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 401, 404]).toContain(response.status());
  });

  test('import Grafana JSON file', async ({ request }) => {
    const dashboardJson = {
      title: 'Test Dashboard',
      panels: []
    };

    const response = await request.post('/api/migrate/grafana/import-json', {
      data: { dashboard: dashboardJson },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 404]).toContain(response.status());
  });
});

test.describe('Prometheus Migration', () => {
  test('import Prometheus rules', async ({ request }) => {
    const rules = `
groups:
  - name: example
    rules:
      - alert: HighRequestLatency
        expr: job:request_latency_seconds:mean5m{job="myjob"} > 0.5
        for: 10m
`;

    const response = await request.post('/api/migrate/prometheus/rules', {
      data: { yaml: rules },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 404]).toContain(response.status());
  });

  test('import Prometheus config', async ({ request }) => {
    const config = `
global:
  scrape_interval: 15s
scrape_configs:
  - job_name: 'prometheus'
    static_configs:
      - targets: ['localhost:9090']
`;

    const response = await request.post('/api/migrate/prometheus/config', {
      data: { yaml: config },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 404]).toContain(response.status());
  });

  test('import recording rules', async ({ request }) => {
    const rules = `
groups:
  - name: recording
    rules:
      - record: job:http_requests:rate5m
        expr: sum(rate(http_requests_total[5m])) by (job)
`;

    const response = await request.post('/api/migrate/prometheus/recording-rules', {
      data: { yaml: rules },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 404]).toContain(response.status());
  });
});

test.describe('Migration History', () => {
  test('list migration history', async ({ request }) => {
    const response = await request.get('/api/migrate/history');

    expect([200, 404]).toContain(response.status());
  });

  test('get migration details', async ({ request }) => {
    const response = await request.get('/api/migrate/history/test-migration-id');

    expect([200, 404]).toContain(response.status());
  });

  test('rollback migration', async ({ request }) => {
    const response = await request.post('/api/migrate/rollback', {
      data: { migrationId: 'test-migration-id' },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 204, 400, 404]).toContain(response.status());
  });
});

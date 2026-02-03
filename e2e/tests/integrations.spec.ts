import { test, expect } from '@playwright/test';

test.describe('Integrations Page', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/integrations.html');
  });

  test('displays all protocol endpoints', async ({ page }) => {
    // Prometheus
    await expect(page.locator('#prometheus-url')).toContainText('/api/v1/write');

    // Graphite
    await expect(page.locator('#graphite-url')).toContainText('/api/graphite/write');

    // InfluxDB
    await expect(page.locator('#influx-url')).toContainText('/api/influx/write');

    // OpenTSDB
    await expect(page.locator('#opentsdb-url')).toContainText('/api/opentsdb/put');

    // StatsD
    await expect(page.locator('#statsd-url')).toContainText('/api/statsd/write');

    // DataDog
    await expect(page.locator('#datadog-url')).toContainText('/api/datadog/v1/series');
  });

  test('copy button works', async ({ page, context }) => {
    // Grant clipboard permissions
    await context.grantPermissions(['clipboard-read', 'clipboard-write']);

    // Click copy on Prometheus URL
    await page.click('#prometheus-url + .copy-btn');

    // Button should show "Copied!"
    await expect(page.locator('#prometheus-url + .copy-btn')).toHaveText('Copied!');

    // Verify clipboard content
    const clipboardText = await page.evaluate(() => navigator.clipboard.readText());
    expect(clipboardText).toContain('/api/v1/write');
  });

  test('example toggle shows/hides content', async ({ page }) => {
    // Find Prometheus example toggle
    const prometheusCard = page.locator('[data-integration="prometheus"]');
    const toggle = prometheusCard.locator('.example-toggle');
    const content = prometheusCard.locator('.example-content');

    // Initially hidden
    await expect(content).not.toHaveClass(/visible/);

    // Click to show
    await toggle.click();
    await expect(content).toHaveClass(/visible/);

    // Click to hide
    await toggle.click();
    await expect(content).not.toHaveClass(/visible/);
  });

  test('test connection button works for DataDog', async ({ page }) => {
    const datadogCard = page.locator('[data-integration="datadog"]');
    const testBtn = datadogCard.locator('.test-btn');
    const result = datadogCard.locator('.test-result');

    // Click test button
    await testBtn.click();

    // Wait for result
    await expect(result).toHaveClass(/visible/, { timeout: 5000 });

    // Should show success (DataDog validate endpoint always returns valid)
    await expect(result).toHaveClass(/success/);
    await expect(result).toContainText(/success/i);
  });

  test('OTLP endpoints are displayed', async ({ page }) => {
    // Check gRPC endpoint
    await expect(page.locator('#otlp-grpc-url')).toContainText(':4317');

    // Check HTTP endpoint
    await expect(page.locator('#otlp-http-url')).toContainText(':4318');
  });

  test('back link navigates to dashboard', async ({ page }) => {
    await page.click('.nav-back');
    await expect(page).toHaveURL('/');
  });
});

test.describe('Integration API Tests', () => {
  test('Graphite endpoint accepts data', async ({ request }) => {
    const response = await request.post('/api/graphite/write', {
      data: `test.metric ${Math.random() * 100} ${Math.floor(Date.now() / 1000)}`,
      headers: { 'Content-Type': 'text/plain' }
    });

    expect(response.status()).toBe(204);
  });

  test('InfluxDB endpoint accepts data', async ({ request }) => {
    const response = await request.post('/api/influx/write?precision=s', {
      data: 'test,source=e2e value=42',
      headers: { 'Content-Type': 'text/plain' }
    });

    expect(response.status()).toBe(204);
  });

  test('OpenTSDB endpoint accepts data', async ({ request }) => {
    const response = await request.post('/api/opentsdb/put', {
      data: JSON.stringify([{
        metric: 'test.e2e',
        timestamp: Math.floor(Date.now() / 1000),
        value: 42,
        tags: { source: 'e2e' }
      }]),
      headers: { 'Content-Type': 'application/json' }
    });

    expect(response.status()).toBe(204);
  });

  test('StatsD endpoint accepts data', async ({ request }) => {
    const response = await request.post('/api/statsd/write', {
      data: 'test.e2e:1|c|#source:e2e',
      headers: { 'Content-Type': 'text/plain' }
    });

    expect(response.status()).toBe(204);
  });

  test('DataDog validate endpoint returns valid', async ({ request }) => {
    const response = await request.get('/api/datadog/v1/validate');

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.valid).toBe(true);
  });

  test('DataDog series endpoint accepts data', async ({ request }) => {
    const response = await request.post('/api/datadog/v1/series', {
      data: JSON.stringify({
        series: [{
          metric: 'test.e2e',
          type: 'gauge',
          points: [[Math.floor(Date.now() / 1000), 42]],
          tags: ['source:e2e']
        }]
      }),
      headers: {
        'Content-Type': 'application/json',
        'DD-API-KEY': 'test-key'
      }
    });

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.status).toBe('ok');
  });
});

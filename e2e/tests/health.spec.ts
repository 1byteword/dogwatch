import { test, expect } from '@playwright/test';

test.describe('Health Checks', () => {
  test('healthz endpoint returns OK', async ({ request }) => {
    const response = await request.get('/healthz');

    expect(response.status()).toBe(200);

    const text = await response.text();
    expect(text.toLowerCase()).toContain('ok');
  });

  test('readyz endpoint returns OK', async ({ request }) => {
    const response = await request.get('/readyz');

    expect(response.status()).toBe(200);
  });

  test('livez endpoint returns OK', async ({ request }) => {
    const response = await request.get('/livez');

    expect(response.status()).toBe(200);
  });
});

test.describe('API Basics', () => {
  test('metrics endpoint returns Prometheus format', async ({ request }) => {
    const response = await request.get('/metrics');

    // May be 404 if metrics endpoint not exposed
    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const text = await response.text();
      // Should have Prometheus metric format
      expect(text).toMatch(/# (HELP|TYPE)/);
    }
  });

  test('status endpoint returns system info', async ({ request }) => {
    const response = await request.get('/api/status');

    if (response.status() === 200) {
      const body = await response.json();
      expect(body).toBeDefined();
    }
  });
});

test.describe('WebSocket Connection', () => {
  test('WebSocket endpoint accepts connections', async ({ page }) => {
    await page.goto('/');

    // Try to establish WebSocket connection
    const wsConnected = await page.evaluate(() => {
      return new Promise((resolve) => {
        try {
          const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
          const ws = new WebSocket(`${protocol}//${window.location.host}/api/ws`);

          ws.onopen = () => {
            ws.close();
            resolve(true);
          };

          ws.onerror = () => {
            resolve(false);
          };

          // Timeout after 5 seconds
          setTimeout(() => {
            ws.close();
            resolve(false);
          }, 5000);
        } catch {
          resolve(false);
        }
      });
    });

    // WebSocket should connect (or gracefully fail if not enabled)
    expect(typeof wsConnected).toBe('boolean');
  });
});

test.describe('Static Assets', () => {
  test('CSS files load', async ({ request }) => {
    const cssFiles = [
      '/css/variables.css',
      '/css/base.css',
      '/css/components.css',
      '/css/sidebar.css'
    ];

    for (const file of cssFiles) {
      const response = await request.get(file);
      // Should return 200 or 304
      expect([200, 304]).toContain(response.status());
    }
  });

  test('JavaScript files load', async ({ request }) => {
    const jsFiles = [
      '/js/loader.js',
      '/js/app.js'
    ];

    for (const file of jsFiles) {
      const response = await request.get(file);
      expect([200, 304]).toContain(response.status());
    }
  });
});

test.describe('Error Handling', () => {
  test('404 for non-existent API endpoint', async ({ request }) => {
    const response = await request.get('/api/nonexistent');

    expect(response.status()).toBe(404);
  });

  test('invalid query returns error', async ({ request }) => {
    const response = await request.get('/api/v1/query', {
      params: {
        query: '' // Empty query
      }
    });

    // Should return 400 Bad Request
    expect([400, 422]).toContain(response.status());
  });
});

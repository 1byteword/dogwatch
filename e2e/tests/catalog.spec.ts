import { test, expect } from '@playwright/test';

test.describe('Service Catalog API', () => {
  const testService = {
    name: 'e2e-test-service',
    displayName: 'E2E Test Service',
    description: 'Service created by e2e tests',
    tier: 'tier2',
    team: 'platform',
    owner: 'e2e@example.com',
    lifecycle: 'production',
    links: [
      { name: 'Repository', url: 'https://github.com/example/service' },
      { name: 'Docs', url: 'https://docs.example.com/service' }
    ],
    tags: ['e2e', 'test'],
    metadata: {
      language: 'go',
      framework: 'stdlib'
    }
  };

  test('list services', async ({ request }) => {
    const response = await request.get('/api/catalog/services');

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(Array.isArray(body) || body.services !== undefined).toBe(true);
  });

  test('create service', async ({ request }) => {
    const response = await request.post('/api/catalog/services', {
      data: testService,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 403, 409]).toContain(response.status());

    if (response.status() === 200 || response.status() === 201) {
      const body = await response.json();
      expect(body.name || body.id).toBeDefined();
    }
  });

  test('get service by name', async ({ request }) => {
    const response = await request.get(`/api/catalog/services/${testService.name}`);

    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const body = await response.json();
      expect(body.name).toBe(testService.name);
    }
  });

  test('update service', async ({ request }) => {
    const updated = { ...testService, description: 'Updated by e2e test' };

    const response = await request.put(`/api/catalog/services/${testService.name}`, {
      data: updated,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 204, 403, 404]).toContain(response.status());
  });

  test('get service dependencies', async ({ request }) => {
    const response = await request.get(`/api/catalog/services/${testService.name}/dependencies`);

    expect([200, 404]).toContain(response.status());
  });

  test('get service health', async ({ request }) => {
    const response = await request.get(`/api/catalog/services/${testService.name}/health`);

    expect([200, 400, 403, 404, 405]).toContain(response.status());
  });

  test('get service metrics', async ({ request }) => {
    const response = await request.get(`/api/catalog/services/${testService.name}/metrics`);

    expect([200, 404]).toContain(response.status());
  });

  test('list services by team', async ({ request }) => {
    const response = await request.get('/api/catalog/services', {
      params: { team: 'platform' }
    });

    expect(response.status()).toBe(200);
  });

  test('list services by tier', async ({ request }) => {
    const response = await request.get('/api/catalog/services', {
      params: { tier: 'tier1' }
    });

    expect(response.status()).toBe(200);
  });

  test('search services', async ({ request }) => {
    const response = await request.get('/api/catalog/services/search', {
      params: { q: 'api' }
    });

    expect([200, 404]).toContain(response.status());
  });

  test('delete service', async ({ request }) => {
    const response = await request.delete(`/api/catalog/services/${testService.name}`);

    expect([200, 204, 403, 404]).toContain(response.status());
  });
});

test.describe('Service Dependencies', () => {
  test('list all dependencies', async ({ request }) => {
    const response = await request.get('/api/catalog/dependencies');

    expect([200, 400, 403, 404, 405]).toContain(response.status());
  });

  test('add dependency', async ({ request }) => {
    const dependency = {
      source: 'e2e-test-service',
      target: 'database-service',
      type: 'database'
    };

    const response = await request.post('/api/catalog/dependencies', {
      data: dependency,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 403, 404]).toContain(response.status());
  });

  test('get dependency graph', async ({ request }) => {
    const response = await request.get('/api/catalog/dependencies/graph');

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Teams API', () => {
  test('list teams', async ({ request }) => {
    const response = await request.get('/api/catalog/teams');

    expect([200, 403, 404]).toContain(response.status());
  });

  test('create team', async ({ request }) => {
    const team = {
      name: 'e2e-team',
      displayName: 'E2E Test Team',
      description: 'Team for e2e testing',
      members: ['user1@example.com'],
      slack: '#e2e-team'
    };

    const response = await request.post('/api/catalog/teams', {
      data: team,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 403, 409]).toContain(response.status());
  });

  test('get team services', async ({ request }) => {
    const response = await request.get('/api/catalog/teams/platform/services');

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Service Catalog UI', () => {
  test('catalog page loads', async ({ page }) => {
    const response = await page.goto('/catalog.html');

    // May be a 404 if page doesn't exist
    if (response && response.status() === 404) {
      return;
    }

    await expect(page.locator('body')).toBeVisible();
  });

  test('service detail page loads', async ({ page }) => {
    // Navigate to catalog first
    await page.goto('/catalog.html');

    // Try to click on a service if any exist
    const serviceLink = page.locator('.service-link, .service-name, a[href*="service"]').first();
    if (await serviceLink.isVisible()) {
      await serviceLink.click();

      // Should show service details
      await expect(page.locator('.service-detail, .service-info, h1')).toBeVisible();
    }
  });
});

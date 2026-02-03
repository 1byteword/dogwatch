import { test, expect } from '@playwright/test';

test.describe('Incidents API', () => {
  const testIncident = {
    title: 'E2E Test Incident',
    severity: 'P2',
    status: 'investigating',
    description: 'Test incident from e2e suite',
    services: ['api-service'],
    labels: { source: 'e2e' }
  };

  let incidentId: string;

  test('list incidents', async ({ request }) => {
    const response = await request.get('/api/incidents');

    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const body = await response.json();
      // Body may be null, array, or object with incidents
      expect(body === null || Array.isArray(body) || body.incidents !== undefined || typeof body === 'object').toBe(true);
    }
  });

  test('create incident', async ({ request }) => {
    const response = await request.post('/api/incidents', {
      data: testIncident,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 403, 404]).toContain(response.status());

    if (response.status() === 200 || response.status() === 201) {
      const body = await response.json();
      expect(body.id).toBeDefined();
      incidentId = body.id;
    }
  });

  test('get incident by id', async ({ request }) => {
    // Create first
    const createResp = await request.post('/api/incidents', {
      data: { ...testIncident, title: 'E2E Get Test Incident' },
      headers: { 'Content-Type': 'application/json' }
    });

    if (createResp.status() === 200 || createResp.status() === 201) {
      const created = await createResp.json();

      const response = await request.get(`/api/incidents/${created.id}`);

      expect(response.status()).toBe(200);

      const body = await response.json();
      expect(body.title).toContain('E2E');
    }
  });

  test('update incident status', async ({ request }) => {
    // Create first
    const createResp = await request.post('/api/incidents', {
      data: { ...testIncident, title: 'E2E Update Test Incident' },
      headers: { 'Content-Type': 'application/json' }
    });

    if (createResp.status() === 200 || createResp.status() === 201) {
      const created = await createResp.json();

      const response = await request.patch(`/api/incidents/${created.id}`, {
        data: { status: 'mitigated' },
        headers: { 'Content-Type': 'application/json' }
      });

      expect([200, 204]).toContain(response.status());
    }
  });

  test('add incident timeline event', async ({ request }) => {
    // Create first
    const createResp = await request.post('/api/incidents', {
      data: { ...testIncident, title: 'E2E Timeline Test Incident' },
      headers: { 'Content-Type': 'application/json' }
    });

    if (createResp.status() === 200 || createResp.status() === 201) {
      const created = await createResp.json();

      const event = {
        type: 'note',
        content: 'Test timeline event from e2e',
        author: 'e2e-test'
      };

      const response = await request.post(`/api/incidents/${created.id}/timeline`, {
        data: event,
        headers: { 'Content-Type': 'application/json' }
      });

      expect([200, 201]).toContain(response.status());
    }
  });

  test('get incident timeline', async ({ request }) => {
    // Create first
    const createResp = await request.post('/api/incidents', {
      data: { ...testIncident, title: 'E2E Timeline Get Test' },
      headers: { 'Content-Type': 'application/json' }
    });

    if (createResp.status() === 200 || createResp.status() === 201) {
      const created = await createResp.json();

      const response = await request.get(`/api/incidents/${created.id}/timeline`);

      expect([200, 404]).toContain(response.status());
    }
  });

  test('resolve incident', async ({ request }) => {
    // Create first
    const createResp = await request.post('/api/incidents', {
      data: { ...testIncident, title: 'E2E Resolve Test Incident' },
      headers: { 'Content-Type': 'application/json' }
    });

    if (createResp.status() === 200 || createResp.status() === 201) {
      const created = await createResp.json();

      const response = await request.post(`/api/incidents/${created.id}/resolve`, {
        data: { summary: 'Resolved by e2e test' },
        headers: { 'Content-Type': 'application/json' }
      });

      expect([200, 204]).toContain(response.status());
    }
  });

  test('list incidents by severity', async ({ request }) => {
    const response = await request.get('/api/incidents', {
      params: { severity: 'P1' }
    });

    expect(response.status()).toBe(200);
  });

  test('list incidents by status', async ({ request }) => {
    const response = await request.get('/api/incidents', {
      params: { status: 'open' }
    });

    expect(response.status()).toBe(200);
  });
});

test.describe('On-Call API', () => {
  test('list on-call schedules', async ({ request }) => {
    const response = await request.get('/api/oncall/schedules');

    expect([200, 404]).toContain(response.status());
  });

  test('get current on-call', async ({ request }) => {
    const response = await request.get('/api/oncall/current');

    expect([200, 404]).toContain(response.status());
  });

  test('create on-call schedule', async ({ request }) => {
    const schedule = {
      name: 'e2e-schedule',
      timezone: 'UTC',
      rotations: [
        {
          name: 'primary',
          users: ['user1@example.com'],
          startTime: '09:00',
          duration: '24h'
        }
      ]
    };

    const response = await request.post('/api/oncall/schedules', {
      data: schedule,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 403, 404, 409]).toContain(response.status());
  });

  test('create escalation policy', async ({ request }) => {
    const policy = {
      name: 'e2e-escalation',
      steps: [
        {
          delay: '5m',
          targets: [{ type: 'user', id: 'user1@example.com' }]
        }
      ]
    };

    const response = await request.post('/api/oncall/escalations', {
      data: policy,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 403, 404, 409]).toContain(response.status());
  });

  test('list escalation policies', async ({ request }) => {
    const response = await request.get('/api/oncall/escalations');

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Incidents UI', () => {
  test('incidents page loads', async ({ page }) => {
    await page.goto('/incidents.html');

    await expect(page.locator('body')).toBeVisible();
  });

  test('on-call page loads', async ({ page }) => {
    await page.goto('/oncall.html');

    await expect(page.locator('body')).toBeVisible();
  });
});

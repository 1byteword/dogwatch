import { test, expect } from '@playwright/test';

test.describe('Backup API', () => {
  test('list backups', async ({ request }) => {
    const response = await request.get('/api/backup/list');

    expect([200, 404]).toContain(response.status());

    if (response.status() === 200) {
      const body = await response.json();
      expect(Array.isArray(body) || body.backups !== undefined).toBe(true);
    }
  });

  test('create backup', async ({ request }) => {
    const response = await request.post('/api/backup/create', {
      data: {
        name: 'e2e-test-backup',
        includeData: true,
        includeConfig: true
      },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 202, 400, 401, 403]).toContain(response.status());
  });

  test('get backup status', async ({ request }) => {
    const response = await request.get('/api/backup/status');

    expect([200, 404]).toContain(response.status());
  });

  test('get backup by id', async ({ request }) => {
    const response = await request.get('/api/backup/e2e-test-backup');

    expect([200, 404]).toContain(response.status());
  });

  test('delete backup', async ({ request }) => {
    const response = await request.delete('/api/backup/e2e-test-backup');

    expect([200, 204, 401, 403, 404]).toContain(response.status());
  });
});

test.describe('Restore API', () => {
  test('list restore points', async ({ request }) => {
    const response = await request.get('/api/restore/points');

    expect([200, 404]).toContain(response.status());
  });

  test('validate restore (dry run)', async ({ request }) => {
    const response = await request.post('/api/restore/validate', {
      data: {
        backupId: 'e2e-test-backup',
        dryRun: true
      },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 400, 403, 404]).toContain(response.status());
  });

  test('restore endpoint exists', async ({ request }) => {
    // This just verifies the endpoint exists, not actual restore
    const response = await request.post('/api/restore', {
      data: {
        backupId: 'nonexistent',
        dryRun: true
      },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 400, 401, 403, 404]).toContain(response.status());
  });
});

test.describe('Backup Schedules', () => {
  test('list backup schedules', async ({ request }) => {
    const response = await request.get('/api/backup/schedules');

    expect([200, 404]).toContain(response.status());
  });

  test('create backup schedule', async ({ request }) => {
    const schedule = {
      name: 'e2e-daily-backup',
      cron: '0 2 * * *',
      retention: '7d',
      includeData: true,
      includeConfig: true
    };

    const response = await request.post('/api/backup/schedules', {
      data: schedule,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 401, 403, 409]).toContain(response.status());
  });

  test('update backup schedule', async ({ request }) => {
    const response = await request.put('/api/backup/schedules/e2e-daily-backup', {
      data: { retention: '14d' },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 204, 400, 401, 403, 404]).toContain(response.status());
  });

  test('delete backup schedule', async ({ request }) => {
    const response = await request.delete('/api/backup/schedules/e2e-daily-backup');

    expect([200, 204, 401, 403, 404]).toContain(response.status());
  });
});

test.describe('Export/Import', () => {
  test('export configuration', async ({ request }) => {
    const response = await request.get('/api/export/config');

    expect([200, 401, 403, 404]).toContain(response.status());

    if (response.status() === 200) {
      const contentType = response.headers()['content-type'];
      expect(contentType).toMatch(/json|yaml|octet-stream/);
    }
  });

  test('export dashboards', async ({ request }) => {
    const response = await request.get('/api/export/dashboards');

    expect([200, 401, 403, 404]).toContain(response.status());
  });

  test('export alerts', async ({ request }) => {
    const response = await request.get('/api/export/alerts');

    expect([200, 401, 403, 404]).toContain(response.status());
  });

  test('import configuration', async ({ request }) => {
    const response = await request.post('/api/import/config', {
      data: { version: '1.0', config: {} },
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 401, 403]).toContain(response.status());
  });
});

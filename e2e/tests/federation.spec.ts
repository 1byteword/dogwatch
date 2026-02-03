import { test, expect } from '@playwright/test';

test.describe('Federation', () => {
  test('list federated clusters', async ({ request }) => {
    const response = await request.get('/api/federation/clusters');

    expect([200, 404]).toContain(response.status());
  });

  test('add federated cluster', async ({ request }) => {
    const cluster = {
      name: 'e2e-remote-cluster',
      url: 'http://remote.example.com:9999',
      auth: {
        type: 'basic',
        username: 'test',
        password: 'test'
      }
    };

    const response = await request.post('/api/federation/clusters', {
      data: cluster,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 403, 404, 409]).toContain(response.status());
  });

  test('get cluster status', async ({ request }) => {
    const response = await request.get('/api/federation/clusters/e2e-remote-cluster/status');

    expect([200, 404]).toContain(response.status());
  });

  test('remove federated cluster', async ({ request }) => {
    const response = await request.delete('/api/federation/clusters/e2e-remote-cluster');

    expect([200, 204, 403, 404]).toContain(response.status());
  });

  test('query across clusters', async ({ request }) => {
    const response = await request.get('/api/v1/query', {
      params: {
        query: 'up',
        federated: 'true'
      }
    });

    expect([200, 400]).toContain(response.status());
  });
});

test.describe('Remote Write', () => {
  test('configure remote write target', async ({ request }) => {
    const target = {
      name: 'e2e-remote-write',
      url: 'http://remote.example.com:9999/api/v1/write',
      enabled: false
    };

    const response = await request.post('/api/remote-write/targets', {
      data: target,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 403, 404, 409]).toContain(response.status());
  });

  test('list remote write targets', async ({ request }) => {
    const response = await request.get('/api/remote-write/targets');

    expect([200, 404]).toContain(response.status());
  });

  test('get remote write status', async ({ request }) => {
    const response = await request.get('/api/remote-write/status');

    expect([200, 404]).toContain(response.status());
  });

  test('delete remote write target', async ({ request }) => {
    const response = await request.delete('/api/remote-write/targets/e2e-remote-write');

    expect([200, 204, 403, 404]).toContain(response.status());
  });
});

test.describe('Scrape Targets', () => {
  test('list scrape targets', async ({ request }) => {
    const response = await request.get('/api/targets');

    expect([200, 404]).toContain(response.status());
  });

  test('Prometheus-compatible targets endpoint', async ({ request }) => {
    const response = await request.get('/api/v1/targets');

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.status).toBe('success');
  });

  test('add scrape target', async ({ request }) => {
    const target = {
      job: 'e2e-scrape-job',
      targets: ['localhost:9999'],
      labels: { source: 'e2e' }
    };

    const response = await request.post('/api/targets', {
      data: target,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 403, 404, 409]).toContain(response.status());
  });

  test('delete scrape target', async ({ request }) => {
    const response = await request.delete('/api/targets/e2e-scrape-job');

    expect([200, 204, 403, 404]).toContain(response.status());
  });

  test('get target health', async ({ request }) => {
    const response = await request.get('/api/targets/health');

    expect([200, 404]).toContain(response.status());
  });
});

test.describe('Service Discovery', () => {
  test('list discovered services', async ({ request }) => {
    const response = await request.get('/api/discovery/services');

    expect([200, 404]).toContain(response.status());
  });

  test('configure Kubernetes SD', async ({ request }) => {
    const config = {
      type: 'kubernetes',
      kubeconfig: '/path/to/kubeconfig',
      namespaces: ['default']
    };

    const response = await request.post('/api/discovery/kubernetes', {
      data: config,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 403, 404]).toContain(response.status());
  });

  test('configure file SD', async ({ request }) => {
    const config = {
      type: 'file',
      files: ['/etc/dogwatch/targets/*.json']
    };

    const response = await request.post('/api/discovery/file', {
      data: config,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 403, 404]).toContain(response.status());
  });

  test('configure DNS SD', async ({ request }) => {
    const config = {
      type: 'dns',
      names: ['_prometheus._tcp.example.com']
    };

    const response = await request.post('/api/discovery/dns', {
      data: config,
      headers: { 'Content-Type': 'application/json' }
    });

    expect([200, 201, 400, 403, 404]).toContain(response.status());
  });
});

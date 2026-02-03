import { test, expect } from '@playwright/test';

test.describe('Query Builder', () => {
  test.beforeEach(async ({ page }) => {
    await page.goto('/query-builder.html');
  });

  test('page loads successfully', async ({ page }) => {
    // Check title
    await expect(page).toHaveTitle(/Query/i);

    // Check for query input area
    await expect(page.locator('.query-input, textarea, input[type="text"]')).toBeVisible();
  });

  test('can select data source', async ({ page }) => {
    // Look for source buttons
    const sourceButtons = page.locator('.source-btn');

    if (await sourceButtons.count() > 0) {
      // Click first source
      await sourceButtons.first().click();

      // Should be active
      await expect(sourceButtons.first()).toHaveClass(/active/);
    }
  });

  test('query input accepts text', async ({ page }) => {
    // Find query input (could be textarea or input)
    const queryInput = page.locator('.query-input, .query-editor textarea, #query-input');

    if (await queryInput.isVisible()) {
      await queryInput.fill('up{job="test"}');

      // Verify input value
      await expect(queryInput).toHaveValue(/up/);
    }
  });
});

test.describe('Query API', () => {
  test('instant query endpoint works', async ({ request }) => {
    const response = await request.get('/api/v1/query', {
      params: {
        query: 'up',
        time: Math.floor(Date.now() / 1000).toString()
      }
    });

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.status).toBe('success');
    expect(body.data).toBeDefined();
    expect(body.data.resultType).toBeDefined();
  });

  test('range query endpoint works', async ({ request }) => {
    const end = Math.floor(Date.now() / 1000);
    const start = end - 3600; // 1 hour ago

    const response = await request.get('/api/v1/query_range', {
      params: {
        query: 'up',
        start: start.toString(),
        end: end.toString(),
        step: '60'
      }
    });

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.status).toBe('success');
    expect(body.data).toBeDefined();
  });

  test('labels endpoint works', async ({ request }) => {
    const response = await request.get('/api/v1/labels');

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.status).toBe('success');
    expect(Array.isArray(body.data)).toBe(true);
  });

  test('series endpoint works', async ({ request }) => {
    const response = await request.get('/api/v1/series', {
      params: {
        'match[]': 'up'
      }
    });

    expect(response.status()).toBe(200);

    const body = await response.json();
    expect(body.status).toBe('success');
  });
});

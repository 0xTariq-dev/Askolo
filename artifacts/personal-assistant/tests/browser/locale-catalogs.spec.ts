import { expect, test, type Page, type Route } from '@playwright/test';

const basePath = '/personal-assistant/';

function jsonResponse(payload: unknown) {
  return {
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(payload),
  };
}

async function installSignedInFixtures(page: Page) {
  let preferredLocale = 'en';

  await page.route('**/api/**', async (route: Route) => {
    const request = route.request();
    const { pathname } = new URL(request.url());

    if (request.method() === 'GET' && pathname.endsWith('/api/auth/user')) {
      await route.fulfill(jsonResponse({
        user: {
          id: 'locale-e2e-user',
          email: 'locale.qa@example.test',
          firstName: 'Locale',
          lastName: 'QA',
          profileImageUrl: '',
          preferredLocale,
          status: 'active',
          emailVerified: true,
          accountCreatedVia: 'email',
          authProvider: 'password',
        },
        mfaRequired: false,
      }));
      return;
    }

    if (request.method() === 'PATCH' && pathname.endsWith('/api/user/profile')) {
      const input = request.postDataJSON() as { preferredLocale?: string };
      if (input.preferredLocale) preferredLocale = input.preferredLocale;
      await route.fulfill(jsonResponse({
        id: 'locale-e2e-user',
        email: 'locale.qa@example.test',
        firstName: 'Locale',
        lastName: 'QA',
        profileImageUrl: '',
        preferredLocale,
      }));
      return;
    }

    if (pathname.endsWith('/api/auth/mfa/status')) {
      await route.fulfill(jsonResponse({ enabled: false }));
      return;
    }
    if (pathname.endsWith('/api/google/status')) {
      await route.fulfill(jsonResponse({
        connected: false,
        scopes: [],
        calendarConnected: false,
        gmailConnected: false,
        gmailActionRequired: false,
        accounts: [],
      }));
      return;
    }
    if (pathname.endsWith('/api/integrations/microsoft/status')) {
      await route.fulfill(jsonResponse({ accounts: [], connected: false }));
      return;
    }
    if (pathname.endsWith('/api/v2/workspaces')) {
      await route.fulfill(jsonResponse([{
        id: 'locale-e2e-space',
        name: 'Locale QA',
        description: null,
        imageObjectPath: null,
        timeZone: 'UTC',
        slug: null,
        isPersonal: true,
        role: 'owner',
      }]));
      return;
    }
    if (pathname.endsWith('/api/v2/notifications')) {
      await route.fulfill(jsonResponse([]));
      return;
    }
    if (pathname.endsWith('/api/ai/credits')) {
      await route.fulfill(jsonResponse({
        currency: 'USD',
        balanceUsdMicros: 0,
        grantedUsdMicros: 0,
        adjustmentsUsdMicros: 0,
        reservedUsdMicros: 0,
        spentUsdMicros: 0,
        refundedUsdMicros: 0,
        policyVersion: 1,
        canManage: false,
        enforcement: 'strict',
      }));
      return;
    }
    if (pathname.endsWith('/api/dashboard/summary')) {
      await route.fulfill(jsonResponse({
        habits: [],
        goals: [],
        todayPlan: [],
        upcomingEvents: [],
        pendingChores: [],
        openActionItems: [],
        habitsCompletedToday: 12,
        habitsTotal: 34,
        goalsActive: 0,
        goalsCompleted: 0,
        googleConnection: {
          connected: false,
          scopes: [],
          calendarConnected: false,
          gmailConnected: false,
          gmailActionRequired: false,
          accounts: [],
        },
      }));
      return;
    }
    if (pathname.endsWith('/agenda')) {
      const url = new URL(request.url());
      await route.fulfill(jsonResponse({
        workspaceId: 'locale-e2e-space',
        from: url.searchParams.get('from'),
        to: url.searchParams.get('to'),
        timeZone: 'UTC',
        tasks: [],
        planEntries: [],
        calendarEvents: [],
      }));
      return;
    }
    if (pathname.endsWith('/api/v2/workspaces/locale-e2e-space/home')) {
      await route.fulfill(jsonResponse({
        date: '2026-11-04',
        timeZone: 'UTC',
        generatedAt: '2026-11-04T12:00:00Z',
        scope: 'workspace',
        focusActions: [],
        emailSummary: {
          unreadInboxCount: 0,
          actionNeededCount: 0,
          lastSyncedAt: null,
          freshness: 'unavailable',
        },
        calendarSummary: { eventCount: 0, nextEvent: null },
      }));
      return;
    }

    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'No locale browser-test fixture for this API request.' }),
    });
  });
}

function trackCatalogResponses(page: Page) {
  const catalogResponses: Array<{ path: string; status: number }> = [];
  page.on('response', (response) => {
    const url = new URL(response.url());
    const match = url.pathname.match(/\/locales\/(en|ar)\/messages\.po$/);
    if (match) catalogResponses.push({ path: match[0], status: response.status() });
  });
  return catalogResponses;
}

test('loads the English catalog in the browser and renders translated landing labels', async ({ page }) => {
  const catalogResponses = trackCatalogResponses(page);
  await page.goto(basePath);

  await expect(page.locator('html')).toHaveAttribute('lang', 'en');
  await expect(page.locator('html')).toHaveAttribute('dir', 'ltr');
  await expect(page.getByRole('banner').getByRole('link', { name: 'Sign In' })).toBeVisible();
  await expect(page.getByRole('banner').getByRole('link', { name: 'Get Started' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Features' })).toBeAttached();
  await expect.poll(() => catalogResponses.some(({ path, status }) =>
    path === '/locales/en/messages.po' && status === 200,
  )).toBe(true);
  await expect(page.getByText(/common\.(?:signIn|getStarted|features)/)).toHaveCount(0);
});

test('loads Arabic after selection and formats dashboard numbers with Arabic-Indic digits', async ({ page }) => {
  await installSignedInFixtures(page);
  const catalogResponses = trackCatalogResponses(page);

  await page.goto(`${basePath}settings/account/profile`);
  await expect(page.getByTestId('select-app-language')).toBeVisible();
  await expect(page.locator('html')).toHaveAttribute('lang', 'en');
  await page.getByTestId('select-app-language').click();
  await page.getByRole('option', { name: 'العربية' }).click();

  await expect(page.locator('html')).toHaveAttribute('lang', 'ar');
  await expect(page.locator('html')).toHaveAttribute('dir', 'rtl');
  await expect.poll(() => catalogResponses.some(({ path, status }) =>
    path === '/locales/ar/messages.po' && status === 200,
  )).toBe(true);

  await page.locator('a[href$="/agenda"]').first().click();
  await page.getByRole('tab', { name: /المهام/ }).click();
  await expect(page.getByText(/٠\s*\/\s*٠\s*مكتملة/)).toBeVisible();
});

test('syncs language, direction, translations, and formatting across tabs without duplicate account writes', async ({ page }) => {
  const secondTab = await page.context().newPage();
  let firstTabProfileWrites = 0;
  let secondTabProfileWrites = 0;
  await page.context().addInitScript(() => {
    window.localStorage.setItem('askolo-locale', 'en');
  });
  page.on('request', (request) => {
    if (request.method() === 'PATCH' && new URL(request.url()).pathname.endsWith('/api/user/profile')) {
      firstTabProfileWrites += 1;
    }
  });
  secondTab.on('request', (request) => {
    if (request.method() === 'PATCH' && new URL(request.url()).pathname.endsWith('/api/user/profile')) {
      secondTabProfileWrites += 1;
    }
  });
  await Promise.all([
    installSignedInFixtures(page),
    installSignedInFixtures(secondTab),
  ]);

  try {
    await Promise.all([
      page.goto(`${basePath}settings/account/profile`),
      secondTab.goto(`${basePath}settings/account/profile`),
    ]);
    await expect(page.getByTestId('select-app-language')).toBeVisible();
    await expect(secondTab.getByTestId('select-app-language')).toBeVisible();
    await expect(page.locator('html')).toHaveAttribute('lang', 'en');
    await expect(secondTab.locator('html')).toHaveAttribute('lang', 'en');

    await page.getByTestId('select-app-language').click();
    await page.getByRole('option', { name: 'العربية' }).click();

    await expect(page.locator('html')).toHaveAttribute('lang', 'ar');
    await expect(page.locator('html')).toHaveAttribute('dir', 'rtl');
    await expect(secondTab.locator('html')).toHaveAttribute('lang', 'ar');
    await expect(secondTab.locator('html')).toHaveAttribute('dir', 'rtl');
    await expect(secondTab.getByTestId('select-app-language')).toContainText('العربية');
    await expect(secondTab.getByText(/اختر اللغة واتجاه القراءة/)).toBeVisible();

    await secondTab.locator('a[href$="/agenda"]').first().click();
    await expect(secondTab.getByRole('heading', { name: 'الأجندة' })).toBeVisible();
    await secondTab.getByRole('tab', { name: /المهام/ }).click();
    await expect(secondTab.getByText(/٠\s*\/\s*٠\s*مكتملة/)).toBeVisible();
    await expect(
      secondTab.locator('section[aria-label="اختيار يوم المهام"] h2'),
    ).toContainText(/[٠-٩]/);

    await expect.poll(() => firstTabProfileWrites).toBe(1);
    expect(secondTabProfileWrites).toBe(0);
  } finally {
    await secondTab.close();
  }
});

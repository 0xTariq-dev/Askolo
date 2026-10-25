// This browser test uses synthetic signed-in/API fixtures. It intentionally
// never authenticates against the Development session or a real Google account.
import { expect, test, type Page, type Route, type TestInfo } from '@playwright/test';

type GoogleAccount = {
  id: string;
  email: string;
  displayName: string;
  status: 'active';
  scopes: string[];
  capabilities: string[];
  updatedAt: string;
};

const basePath = '/personal-assistant/';
const profilePath = `${basePath}settings/account/profile`;
const firstAccount: GoogleAccount = {
  id: 'google-calendar-fixture',
  email: 'calendar.qa@example.test',
  displayName: 'Calendar QA',
  status: 'active',
  scopes: [
    'https://www.googleapis.com/auth/calendar.events',
    'https://www.googleapis.com/auth/gmail.readonly',
  ],
  capabilities: ['calendar', 'gmail.read'],
  updatedAt: '2026-10-25T09:00:00Z',
};
const secondAccount: GoogleAccount = {
  id: 'google-calendar-secondary-fixture',
  email: 'calendar.secondary@example.test',
  displayName: 'Calendar Secondary QA',
  status: 'active',
  scopes: ['https://www.googleapis.com/auth/calendar.events'],
  capabilities: ['calendar'],
  updatedAt: '2026-10-25T09:05:00Z',
};

function jsonResponse(payload: unknown) {
  return {
    status: 200,
    contentType: 'application/json',
    body: JSON.stringify(payload),
  };
}

async function installSignedInFixtures(page: Page) {
  let accounts = [firstAccount];
  const requestedScopes: string[] = [];
  const revokedIds: string[] = [];
  const unexpectedApiRequests: string[] = [];

  const statusResponse = () => ({
    connected: accounts.length > 0,
    scopes: accounts.flatMap((account) => account.scopes),
    calendarConnected: accounts.some((account) => account.capabilities.includes('calendar')),
    gmailConnected: accounts.some((account) => account.capabilities.includes('gmail.read')),
    gmailActionRequired: false,
    accounts,
  });

  await page.route('**/api/**', async (route: Route) => {
    const request = route.request();
    const url = new URL(request.url());
    const method = request.method();
    const path = url.pathname;

    if (method === 'GET' && path.endsWith('/api/auth/user')) {
      await route.fulfill(jsonResponse({
        user: {
          id: 'profile-e2e-user',
          email: 'profile.qa@example.test',
          firstName: 'Profile',
          lastName: 'QA',
          profileImageUrl: '',
          preferredLocale: 'en',
          status: 'active',
          emailVerified: true,
          accountCreatedVia: 'email',
          authProvider: 'password',
        },
        mfaRequired: false,
      }));
      return;
    }

    if (method === 'GET' && path.endsWith('/api/google/status')) {
      await route.fulfill(jsonResponse(statusResponse()));
      return;
    }

    if (method === 'GET' && path.endsWith('/api/auth/mfa/status')) {
      await route.fulfill(jsonResponse({ enabled: false }));
      return;
    }

    if (method === 'GET' && path.endsWith('/api/v2/workspaces')) {
      await route.fulfill(jsonResponse([{
        id: 'profile-e2e-space',
        name: 'Profile QA',
        description: null,
        imageObjectPath: null,
        timeZone: 'UTC',
        slug: null,
        isPersonal: true,
        role: 'owner',
      }]));
      return;
    }

    if (method === 'GET' && path.endsWith('/api/v2/notifications')) {
      await route.fulfill(jsonResponse([]));
      return;
    }

    if (method === 'GET' && path.endsWith('/api/ai/credits')) {
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

    if (method === 'GET' && path.endsWith('/api/google/gmail/connect')) {
      const scope = url.searchParams.get('scope') ?? '';
      requestedScopes.push(scope);
      if (scope !== 'calendar') {
        await route.fulfill({ status: 400, body: 'Unexpected scope in browser test.' });
        return;
      }

      accounts = [firstAccount, secondAccount];
      await route.fulfill({
        status: 302,
        headers: { location: `${profilePath}?test_oauth=complete` },
      });
      return;
    }

    if (method === 'DELETE' && path.startsWith('/api/integrations/google/')) {
      const connectionId = decodeURIComponent(path.split('/').at(-1) ?? '');
      revokedIds.push(connectionId);
      accounts = accounts.filter((account) => account.id !== connectionId);
      await route.fulfill({ status: 204 });
      return;
    }

    unexpectedApiRequests.push(`${method} ${path}`);
    await route.fulfill({
      status: 404,
      contentType: 'application/json',
      body: JSON.stringify({ error: 'No fixture for this API request.' }),
    });
  });

  return { requestedScopes, revokedIds, unexpectedApiRequests };
}

async function runAccountManagementFlow(page: Page, testInfo: TestInfo) {
  const fixture = await installSignedInFixtures(page);
  const runtimeErrors: string[] = [];
  page.on('pageerror', (error) => runtimeErrors.push(error.message));

  await page.goto(profilePath);
  await expect(page.getByText('Google Connections', { exact: true })).toBeVisible();

  const accountSection = page.getByRole('region', { name: 'Connected Google accounts' });
  const firstRow = accountSection.getByRole('listitem').filter({
    hasText: firstAccount.email,
  });
  await expect(firstRow).toContainText('Calendar');
  await expect(firstRow).toContainText('Gmail');

  const addAccountButtons = page.getByRole('button', { name: 'Add account' });
  await expect(addAccountButtons).toHaveCount(2);
  await addAccountButtons.first().click();

  const secondRow = accountSection.getByRole('listitem').filter({
    hasText: secondAccount.email,
  });
  await expect(secondRow).toContainText('Calendar');
  await expect(secondRow).not.toContainText('Gmail');
  await expect(accountSection.getByRole('listitem')).toHaveCount(2);
  expect(fixture.requestedScopes).toEqual(['calendar']);
  await accountSection.screenshot({ path: testInfo.outputPath('two-google-accounts.png') });

  await firstRow.getByRole('button', { name: 'Disconnect account' }).click();
  const confirmation = page.getByRole('alertdialog');
  await expect(confirmation).toContainText(firstAccount.email);
  await confirmation.getByRole('button', { name: 'Disconnect account' }).click();

  await expect(accountSection.getByText(firstAccount.email)).toHaveCount(0);
  await expect(accountSection.getByText(secondAccount.email)).toBeVisible();
  await expect(accountSection.getByRole('listitem')).toHaveCount(1);
  await expect(page.getByRole('button', { name: 'Connect', exact: true })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Add account', exact: true })).toBeVisible();
  expect(fixture.revokedIds).toEqual([firstAccount.id]);
  expect(fixture.unexpectedApiRequests).toEqual([]);
  expect(runtimeErrors).toEqual([]);
  await accountSection.screenshot({ path: testInfo.outputPath('remaining-google-account.png') });
}

for (const viewport of [
  { name: 'desktop', size: { width: 1440, height: 1000 } },
  { name: 'mobile', size: { width: 390, height: 844 } },
]) {
  test.describe(`Profile Google account management at ${viewport.name} width`, () => {
    test.use({ viewport: viewport.size });

    test('adds an account, displays per-account capabilities, then revokes only the selected account', async ({ page }, testInfo) => {
      await runAccountManagementFlow(page, testInfo);
      const disconnectServiceButton = page.getByRole('button', { name: 'Disconnect service' });
      await disconnectServiceButton.scrollIntoViewIfNeeded();
      const buttonBox = await disconnectServiceButton.boundingBox();
      const viewportWidth = await page.evaluate(() => window.innerWidth);
      expect(buttonBox).not.toBeNull();
      expect(buttonBox!.x + buttonBox!.width).toBeLessThanOrEqual(viewportWidth);
      const hasHorizontalOverflow = await page.evaluate(
        () => document.documentElement.scrollWidth > window.innerWidth,
      );
      expect(hasHorizontalOverflow).toBe(false);
    });
  });
}

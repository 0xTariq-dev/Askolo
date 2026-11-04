import { expect, test, type Page, type Route } from '@playwright/test';

type Flow = 'login' | 'signup' | 'passwordRecovery' | 'emailResend' | 'mfaRecovery';
type FailureCallback = 'expired-callback' | 'error-callback';

type TurnstileTestDriver = {
  latestWidget: () => { id: string; action: string } | null;
  fireLatest: (callback: 'callback' | FailureCallback, token?: string) => void;
  resetCalls: () => string[];
};

declare global {
  interface Window {
    __askoloTurnstileTest?: TurnstileTestDriver;
  }
}

type ProtectedRequest = {
  path: string;
  body: Record<string, unknown>;
};

const basePath = '/personal-assistant/';

const flows: Array<{
  name: Flow;
  action: string;
  endpoint: string;
  entryPath: string;
}> = [
  { name: 'login', action: 'login', endpoint: '/api/auth/password/login', entryPath: 'sign-in' },
  { name: 'signup', action: 'signup', endpoint: '/api/auth/password/signup', entryPath: 'sign-up' },
  {
    name: 'passwordRecovery',
    action: 'password_recovery',
    endpoint: '/api/auth/password/recovery/request',
    entryPath: 'sign-in',
  },
  {
    name: 'emailResend',
    action: 'email_resend',
    endpoint: '/api/auth/email/resend',
    entryPath: 'sign-up',
  },
  {
    name: 'mfaRecovery',
    action: 'mfa_recovery',
    endpoint: '/api/auth/mfa/recovery/request',
    entryPath: 'sign-in',
  },
];

const failureCases: Array<{ callback: FailureCallback; message: string }> = [
  {
    callback: 'expired-callback',
    message: 'The security check expired. Complete it again before continuing.',
  },
  {
    callback: 'error-callback',
    message: 'The security check could not be completed. Try again.',
  },
];

async function installTurnstileAndAuthFixtures(
  page: Page,
  requests: ProtectedRequest[],
  mfaRequired = false,
) {
  await page.addInitScript(() => {
    type Options = {
      action: string;
      callback: (token: string) => void;
      'error-callback': () => void;
      'expired-callback': () => void;
      'timeout-callback': () => void;
    };

    const widgets: Array<{ id: string; options: Options; marker: HTMLElement }> = [];
    const resets: string[] = [];
    let nextId = 0;

    const turnstile = {
      render(container: HTMLElement, options: Options) {
        const id = `test-turnstile-${++nextId}`;
        const marker = document.createElement('div');
        marker.dataset.testTurnstileWidget = id;
        container.append(marker);
        widgets.push({ id, options, marker });
        return id;
      },
      reset(id?: string) {
        if (id) resets.push(id);
      },
      remove(id: string) {
        const widget = widgets.find((entry) => entry.id === id);
        widget?.marker.remove();
      },
    };

    Object.defineProperty(window, 'turnstile', {
      configurable: true,
      value: turnstile,
    });
    Object.defineProperty(window, '__askoloTurnstileTest', {
      configurable: true,
      value: {
        latestWidget: () => {
          const widget = widgets.at(-1);
          return widget ? { id: widget.id, action: widget.options.action } : null;
        },
        fireLatest: (callback: 'callback' | FailureCallback, token?: string) => {
          const widget = widgets.at(-1);
          if (!widget) throw new Error('No Turnstile widget has rendered.');
          if (callback === 'callback') {
            widget.options.callback(token ?? '');
          } else {
            widget.options[callback]();
          }
        },
        resetCalls: () => [...resets],
      } satisfies TurnstileTestDriver,
    });
  });

  await page.route('**/api/**', async (route: Route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() === 'GET' && path.endsWith('/api/auth/user')) {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ user: null, mfaRequired }),
      });
      return;
    }

    if (request.method() === 'POST') {
      requests.push({
        path,
        body: (request.postDataJSON() ?? {}) as Record<string, unknown>,
      });
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify(
          path.endsWith('/api/auth/password/login')
            ? { status: 'mfa_required' }
            : { status: 'accepted' },
        ),
      });
      return;
    }

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: '{}',
    });
  });
}

async function waitForWidgetAction(page: Page, action: string) {
  await expect.poll(
    () => page.evaluate(() => window.__askoloTurnstileTest?.latestWidget()?.action ?? null),
  ).toBe(action);
}

async function enterFlow(page: Page, flow: Flow, requests: ProtectedRequest[]) {
  const config = flows.find((candidate) => candidate.name === flow)!;
  await installTurnstileAndAuthFixtures(page, requests, flow === 'mfaRecovery');

  if (flow === 'emailResend') {
    await page.clock.install();
  }

  await page.goto(`${basePath}${config.entryPath}`);

  if (flow === 'login') {
    await page.getByLabel('Email address').fill('login@example.test');
    await page.getByLabel('Password', { exact: true }).fill('ValidPass!123');
  } else if (flow === 'signup' || flow === 'emailResend') {
    await page.getByLabel('Email address').fill(`${flow}@example.test`);
    await page.getByLabel('Password', { exact: true }).fill('ValidPass!123');
    await page.getByLabel('Confirm password').fill('ValidPass!123');
  } else if (flow === 'passwordRecovery') {
    await page.getByRole('button', { name: 'Forgot password?' }).click();
    await page.getByLabel('Email address').fill('recovery@example.test');
  } else {
    await page.getByRole('button', { name: 'Start MFA recovery' }).click();
    await page.getByLabel('Email address').fill('mfa-recovery@example.test');
    await page.getByLabel('Account password').fill('ValidPass!123');
  }

  if (flow === 'emailResend') {
    await waitForWidgetAction(page, 'signup');
    await page.evaluate(() => window.__askoloTurnstileTest?.fireLatest('callback', 'signup-setup-token'));
    await page.getByRole('button', { name: 'Create account' }).click();
    await expect(page.getByRole('button', { name: 'Resend verification code' })).toBeVisible();
    await waitForWidgetAction(page, config.action);
    await page.clock.fastForward(121_000);
    await expect(page.getByRole('button', { name: 'Resend verification code' })).toBeEnabled();
    requests.length = 0;
  } else {
    await waitForWidgetAction(page, config.action);
  }
}

async function submitProtectedFlow(page: Page, flow: Flow) {
  const buttonNames: Record<Flow, string> = {
    login: 'Sign in',
    signup: 'Create account',
    passwordRecovery: 'Send recovery code',
    emailResend: 'Resend verification code',
    mfaRecovery: 'Send recovery code',
  };
  await page.getByRole('button', { name: buttonNames[flow] }).click();
}

for (const flow of flows) {
  for (const failure of failureCases) {
    test(`${flow.name} blocks ${failure.callback} tokens and recovers with the same action`, async ({ page }) => {
      const requests: ProtectedRequest[] = [];
      await enterFlow(page, flow.name, requests);

      const beforeFailure = await page.evaluate(
        () => window.__askoloTurnstileTest?.latestWidget() ?? null,
      );
      expect(beforeFailure).not.toBeNull();
      expect(beforeFailure?.action).toBe(flow.action);

      const staleToken = `${flow.name}-stale-token`;
      await page.evaluate(
        (token) => window.__askoloTurnstileTest?.fireLatest('callback', token),
        staleToken,
      );
      await expect(page.getByRole('status').filter({ hasText: 'Security check complete.' }))
        .toBeVisible();

      await page.evaluate(
        (callback) => window.__askoloTurnstileTest?.fireLatest(callback),
        failure.callback,
      );
      await expect(page.getByRole('alert').filter({ hasText: failure.message })).toBeVisible();
      const retryButton = page.getByRole('button', { name: 'Try the security check again' });
      await expect(retryButton).toBeVisible();

      await submitProtectedFlow(page, flow.name);
      await expect.poll(() => requests.filter((request) => request.path.endsWith(flow.endpoint)).length)
        .toBe(0);

      const currentUrl = page.url();
      const resetCount = await page.evaluate(
        () => window.__askoloTurnstileTest?.resetCalls().length ?? 0,
      );
      await retryButton.click();
      await expect.poll(
        () => page.evaluate(() => window.__askoloTurnstileTest?.resetCalls().length ?? 0),
      ).toBe(resetCount + 1);
      expect(page.url()).toBe(currentUrl);

      const afterRetry = await page.evaluate(
        () => window.__askoloTurnstileTest?.latestWidget() ?? null,
      );
      expect(afterRetry?.action).toBe(flow.action);

      const freshToken = `${flow.name}-fresh-token`;
      await page.evaluate(
        (token) => window.__askoloTurnstileTest?.fireLatest('callback', token),
        freshToken,
      );
      await expect(page.getByRole('status').filter({ hasText: 'Security check complete.' }))
        .toBeVisible();

      await submitProtectedFlow(page, flow.name);
      await expect.poll(() => requests.filter((request) => request.path.endsWith(flow.endpoint)).length)
        .toBe(1);
      const protectedRequest = requests.find((request) => request.path.endsWith(flow.endpoint));
      expect(protectedRequest?.body.turnstileToken).toBe(freshToken);
      expect(protectedRequest?.body.turnstileToken).not.toBe(staleToken);
    });
  }
}

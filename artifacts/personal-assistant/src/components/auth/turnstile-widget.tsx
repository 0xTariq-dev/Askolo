import { useEffect, useRef, useState } from 'react';
import { TURNSTILE_SITE_KEY, type TurnstileAction } from '@/lib/auth-turnstile';

type TurnstileOptions = {
  sitekey: string;
  action: TurnstileAction;
  callback: (token: string) => void;
  'error-callback': () => void;
  'expired-callback': () => void;
  'timeout-callback': () => void;
};

type TurnstileAPI = {
  render: (container: HTMLElement, options: TurnstileOptions) => string;
  reset: (widgetId?: string) => void;
  remove: (widgetId: string) => void;
};

declare global {
  interface Window {
    turnstile?: TurnstileAPI;
  }
}

let turnstileScriptPromise: Promise<TurnstileAPI> | null = null;

function loadTurnstile(): Promise<TurnstileAPI> {
  if (window.turnstile) return Promise.resolve(window.turnstile);
  if (turnstileScriptPromise) return turnstileScriptPromise;

  turnstileScriptPromise = new Promise<TurnstileAPI>((resolve, reject) => {
    const script = document.createElement('script');
    script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit';
    script.async = true;
    script.defer = true;
    script.dataset.askoloTurnstile = 'true';
    const timeout = window.setTimeout(() => {
      script.remove();
      reject(new Error('Turnstile script timed out.'));
    }, 15_000);
    script.addEventListener('load', () => {
      window.clearTimeout(timeout);
      if (window.turnstile) {
        resolve(window.turnstile);
      } else {
        script.remove();
        reject(new Error('Turnstile did not initialize.'));
      }
    }, { once: true });
    script.addEventListener('error', () => {
      window.clearTimeout(timeout);
      script.remove();
      reject(new Error('Turnstile could not be loaded.'));
    }, { once: true });
    document.head.appendChild(script);
  }).catch((error: unknown) => {
    turnstileScriptPromise = null;
    throw error;
  });

  return turnstileScriptPromise;
}

type TurnstileWidgetProps = {
  action: TurnstileAction;
  tokenReady: boolean;
  onToken: (action: TurnstileAction, token: string | null) => void;
};

export function TurnstileWidget({ action, tokenReady, onToken }: TurnstileWidgetProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const widgetRef = useRef<{ api: TurnstileAPI; id: string } | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [retryAttempt, setRetryAttempt] = useState(0);

  useEffect(() => {
    let cancelled = false;
    let renderedWidget: { api: TurnstileAPI; id: string } | null = null;
    setError(null);

    void loadTurnstile()
      .then((api) => {
        if (cancelled || !containerRef.current) return;
        const id = api.render(containerRef.current, {
          sitekey: TURNSTILE_SITE_KEY,
          action,
          callback: (token) => {
            setError(null);
            onToken(action, token);
          },
          'error-callback': () => {
            setError('The security check could not be completed. Try again.');
            onToken(action, null);
          },
          'expired-callback': () => {
            setError('The security check expired. Complete it again before continuing.');
            onToken(action, null);
          },
          'timeout-callback': () => {
            setError('The security check timed out. Complete it again before continuing.');
            onToken(action, null);
          },
        });
        renderedWidget = { api, id };
        widgetRef.current = renderedWidget;
      })
      .catch(() => {
        if (cancelled) return;
        setError('The security check could not be loaded. Try again.');
        onToken(action, null);
      });

    return () => {
      cancelled = true;
      if (renderedWidget) {
        renderedWidget.api.remove(renderedWidget.id);
        if (widgetRef.current?.id === renderedWidget.id) widgetRef.current = null;
      }
    };
  }, [action, onToken, retryAttempt]);

  const retry = () => {
    setError(null);
    onToken(action, null);
    const widget = widgetRef.current;
    if (widget) {
      widget.api.reset(widget.id);
    } else {
      setRetryAttempt((attempt) => attempt + 1);
    }
  };

  return (
    <div className="space-y-2" role="group" aria-label="Security verification">
      <p className="text-sm text-muted-foreground">
        Complete the security check before continuing.
      </p>
      <div ref={containerRef} className="min-h-[65px]" />
      {tokenReady && !error && (
        <p className="text-sm text-muted-foreground" role="status" aria-live="polite">
          Security check complete.
        </p>
      )}
      {error && (
        <div className="space-y-1">
          <p className="text-sm text-destructive" role="alert">{error}</p>
          <button
            type="button"
            className="min-h-11 text-sm font-medium text-primary underline underline-offset-4"
            onClick={retry}
          >
            Try the security check again
          </button>
        </div>
      )}
    </div>
  );
}

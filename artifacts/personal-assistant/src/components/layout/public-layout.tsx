import { ReactNode } from 'react';
import { Link } from 'wouter';
import { Button } from '@/components/ui/button';
import { useAskoloTheme } from '@/lib/theme';
import { cn } from '@/lib/utils';
import { Moon, Puzzle, Sparkles, Sun } from 'lucide-react';
import { ExpandedTabsNav } from '@/components/layout/expanded-tabs-nav';
import logoUrl from '/logo.png';
import { toAppUrl } from '@/lib/site-domains';
import { provenanceLabel } from '@/lib/runtime-environment';
import { useLocale } from '@/contexts/locale-context';

export function PublicLayout({
  children,
  background,
}: {
  children: ReactNode;
  background?: ReactNode;
}) {
  const {
    preferences: { mode },
    setMode,
  } = useAskoloTheme();
  const { t } = useLocale();

  return (
    <div className="relative flex min-h-dvh w-full flex-col overflow-x-clip bg-background text-foreground">
      {background ?? (
        <>
          <div className="pointer-events-none absolute start-[-10%] top-0 hidden h-[60%] w-[60%] rounded-full bg-primary/10 blur-[120px] sm:block" />
          <div className="pointer-events-none absolute bottom-[-10%] end-[-10%] hidden h-[50%] w-[50%] rounded-full bg-primary/10 blur-[120px] sm:block" />
        </>
      )}

      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:start-4 focus:top-4 focus:z-[60] focus:rounded-md focus:bg-background focus:px-4 focus:py-2 focus:text-foreground focus:shadow-md"
      >
        {t('nav.skipToMain')}
      </a>

      {/* Header */}
      <header className="sticky top-0 z-50 w-full px-3 pt-3">
        <div className="mx-auto flex h-20 w-full max-w-6xl items-center justify-between rounded-full border border-border/60 bg-background/35 px-4 shadow-sm backdrop-blur-md sm:px-6 md:grid md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
          <Link href="/" aria-label="Askolo" className="group flex shrink-0 items-center justify-self-start">
            <img src={logoUrl} alt="Askolo" width={64} height={64} className="h-10 w-auto object-contain" />
          </Link>
          <ExpandedTabsNav
            ariaLabel="Page sections"
            tabs={[
              { title: t('common.features'), href: '#features', icon: Sparkles },
              { title: t('common.integrations'), href: '#integrations', icon: Puzzle },
            ]}
            className="hidden md:flex"
          />
          <div className="flex shrink-0 items-center justify-self-end gap-1 sm:gap-2">
            <Button variant="ghost" size="sm" asChild className="rounded-full">
              <a href={toAppUrl('/sign-in', { theme: mode })}>{t('common.signIn')}</a>
            </Button>
            <Button size="sm" asChild className="rounded-full">
              <a href={toAppUrl('/sign-up', { theme: mode })}>{t('common.getStarted')}</a>
            </Button>
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="ms-1 min-h-10 min-w-10 shrink-0 rounded-full"
              aria-label={t(mode === 'dark' ? 'common.switchToLight' : 'common.switchToDark')}
              aria-pressed={mode === 'dark'}
              title={t(mode === 'dark' ? 'common.switchToLight' : 'common.switchToDark')}
              onClick={() => setMode(mode === 'dark' ? 'light' : 'dark')}
            >
              <span aria-hidden="true" className="relative size-4">
                <Sun
                  className={cn(
                    'absolute inset-0 m-auto size-4 transition-all duration-300 ease-out motion-reduce:transition-none',
                    mode === 'dark'
                      ? 'scale-0 -rotate-90 opacity-0'
                      : 'scale-100 rotate-0 opacity-100',
                  )}
                />
                <Moon
                  className={cn(
                    'absolute inset-0 m-auto size-4 transition-all duration-300 ease-out motion-reduce:transition-none',
                    mode === 'dark'
                      ? 'scale-100 rotate-0 opacity-100'
                      : 'scale-0 rotate-90 opacity-0',
                  )}
                />
              </span>
              <span className="sr-only">Toggle theme</span>
            </Button>
          </div>
        </div>
      </header>

      {/* Main content */}
      <main id="main-content" tabIndex={-1} className="relative z-10 flex-1">
        {children}
      </main>

      {/* Footer */}
      <footer className="relative z-10 mt-auto border-t border-border/60 bg-background/35 py-8 shadow-sm backdrop-blur-md">
        <div className="max-w-6xl mx-auto px-4 sm:px-6 flex flex-col sm:flex-row gap-4 sm:gap-6 justify-between items-center text-sm text-muted-foreground">
          <div className="flex items-center gap-3">
            <img src={logoUrl} alt="" width={64} height={64} className="h-8 w-auto object-contain" />
            <span>© {new Date().getFullYear()} Askolo</span>
            <span aria-label="Build provenance" className="text-xs opacity-70">
              {provenanceLabel()}
            </span>
          </div>
          <nav aria-label="Footer" className="flex items-center gap-4 sm:gap-6">
            <Link href="/privacy" className="hover:text-primary transition-colors">{t('common.privacyPolicy')}</Link>
            <Link href="/terms" className="hover:text-primary transition-colors">{t('common.termsOfService')}</Link>
          </nav>
        </div>
      </footer>
    </div>
  );
}

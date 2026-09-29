import { ReactNode } from 'react';
import { Link } from 'wouter';
import { Button } from '@workspace/askolo-design-system/components/ui/button';
import { useAskoloTheme } from '@workspace/askolo-design-system/theme';
import { cn } from '@workspace/askolo-design-system/lib/utils';
import { Moon, Sun } from 'lucide-react';
import { AnimatedBrandName } from '@/components/animated-brand-name';
import logoUrl from '/logo.png';
import { toAppUrl } from '@/lib/site-domains';
import { provenanceLabel } from '@/lib/runtime-environment';

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

  return (
    <div className="relative flex min-h-dvh w-full flex-col overflow-x-clip bg-background text-foreground">
      {background ?? (
        <>
          <div className="pointer-events-none absolute left-[-10%] top-0 hidden h-[60%] w-[60%] rounded-full bg-primary/10 blur-[120px] sm:block" />
          <div className="pointer-events-none absolute bottom-[-10%] right-[-10%] hidden h-[50%] w-[50%] rounded-full bg-primary/10 blur-[120px] sm:block" />
        </>
      )}

      <a
        href="#main-content"
        className="sr-only focus:not-sr-only focus:fixed focus:left-4 focus:top-4 focus:z-[60] focus:rounded-md focus:bg-background focus:px-4 focus:py-2 focus:text-foreground focus:shadow-md"
      >
        Skip to main content
      </a>

      {/* Header */}
      <header className="sticky top-0 z-50 w-full border-b border-border/60 bg-background/45 shadow-sm backdrop-blur-md">
        <div className="mx-auto flex h-20 w-full max-w-6xl items-center justify-between px-4 sm:px-6 md:grid md:grid-cols-[minmax(0,1fr)_auto_minmax(0,1fr)]">
          <Link href="/" className="group flex shrink-0 items-center gap-3 justify-self-start">
            <img src={logoUrl} alt="" width={64} height={64} className="h-10 w-auto object-contain" />
            <AnimatedBrandName className="text-2xl" />
          </Link>
          <nav aria-label="Page sections" className="hidden items-center justify-center gap-8 md:flex">
            <a
              href="#features"
              className="rounded-sm px-1 py-2 text-[17px] font-bold text-muted-foreground transition-[color,transform] duration-200 ease-out hover:-translate-y-0.5 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 motion-reduce:transform-none motion-reduce:transition-none"
            >
              Features
            </a>
            <a
              href="#integrations"
              className="rounded-sm px-1 py-2 text-[17px] font-bold text-muted-foreground transition-[color,transform] duration-200 ease-out hover:-translate-y-0.5 hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 motion-reduce:transform-none motion-reduce:transition-none"
            >
              Integrations
            </a>
          </nav>
          <div className="flex shrink-0 items-center justify-self-end gap-1 sm:gap-2">
            <Button variant="ghost" size="sm" asChild className="rounded-full">
              <a href={toAppUrl('/sign-in', { theme: mode })}>Sign In</a>
            </Button>
            <Button size="sm" asChild className="rounded-full">
              <a href={toAppUrl('/sign-up', { theme: mode })}>Get Started</a>
            </Button>
            <Button
              type="button"
              variant="outline"
              size="icon"
              className="ml-1 min-h-10 min-w-10 shrink-0 rounded-full"
              aria-label={`Switch to ${mode === 'dark' ? 'light' : 'dark'} appearance`}
              aria-pressed={mode === 'dark'}
              title={`Switch to ${mode === 'dark' ? 'light' : 'dark'} appearance`}
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
      <footer className="relative z-10 mt-auto border-t border-border/60 bg-background/45 py-8 shadow-sm backdrop-blur-md">
        <div className="max-w-6xl mx-auto px-4 sm:px-6 flex flex-col sm:flex-row gap-4 sm:gap-6 justify-between items-center text-sm text-muted-foreground">
          <div className="flex items-center gap-3">
            <img src={logoUrl} alt="" width={64} height={64} className="h-8 w-auto object-contain" />
            <span>© {new Date().getFullYear()} Askolo</span>
            <span aria-label="Build provenance" className="text-xs opacity-70">
              {provenanceLabel()}
            </span>
          </div>
          <nav aria-label="Footer" className="flex items-center gap-4 sm:gap-6">
            <Link href="/privacy" className="hover:text-primary transition-colors">Privacy Policy</Link>
            <Link href="/terms" className="hover:text-primary transition-colors">Terms of Service</Link>
          </nav>
        </div>
      </footer>
    </div>
  );
}

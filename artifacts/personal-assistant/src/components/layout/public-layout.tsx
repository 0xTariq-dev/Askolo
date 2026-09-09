import { ReactNode } from 'react';
import { Link } from 'wouter';
import { useAuth } from '@clerk/react';
import { Button } from '@/components/ui/button';
import logoUrl from '/logo.png';

const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');

export function PublicLayout({ children }: { children: ReactNode }) {
  const { isSignedIn } = useAuth();

  return (
    <div className="min-h-screen w-full flex flex-col bg-background text-foreground relative overflow-hidden">
      {/* Ambient background glows */}
      <div className="absolute top-[0%] left-[-10%] w-[60%] h-[60%] bg-primary/10 rounded-full blur-[120px] pointer-events-none" />
      <div className="absolute bottom-[-10%] right-[-10%] w-[50%] h-[50%] bg-blue-600/10 rounded-full blur-[120px] pointer-events-none" />

      {/* Header */}
      <header className="relative z-10 w-full border-b border-border/60 bg-background/80 backdrop-blur-sm">
        <div className="max-w-6xl mx-auto px-4 sm:px-6 h-16 flex items-center justify-between">
          <Link href="/" className="flex items-center gap-2 group">
            <img src={logoUrl} alt="Askolo" width={64} height={64} className="h-8 w-auto object-contain" />
            <span className="font-display font-bold text-xl tracking-tight group-hover:text-primary transition-colors">
              Askolo
            </span>
          </Link>
          <nav aria-label="Primary" className="flex items-center gap-2 sm:gap-4">
            <Link href="/privacy" className="text-sm text-muted-foreground hover:text-foreground transition-colors hidden sm:inline">
              Privacy
            </Link>
            <Link href="/terms" className="text-sm text-muted-foreground hover:text-foreground transition-colors hidden sm:inline">
              Terms
            </Link>
            {isSignedIn ? (
              <Button asChild size="sm">
                <Link href="/dashboard">Go to Dashboard</Link>
              </Button>
            ) : (
              <>
                <Button variant="ghost" size="sm" asChild>
                  <Link href="/sign-in">Sign In</Link>
                </Button>
                <Button size="sm" asChild>
                  <Link href="/sign-up">Get Started</Link>
                </Button>
              </>
            )}
          </nav>
        </div>
      </header>

      {/* Main content */}
      <main id="main-content" className="relative z-10 flex-1">{children}</main>

      {/* Footer */}
      <footer className="relative z-10 border-t border-border/60 bg-background/80 backdrop-blur-sm py-8">
        <div className="max-w-6xl mx-auto px-4 sm:px-6 flex flex-col sm:flex-row gap-4 sm:gap-6 justify-between items-center text-sm text-muted-foreground">
          <div className="flex items-center gap-2">
            <img src={logoUrl} alt="Askolo" width={64} height={64} className="h-5 w-auto object-contain opacity-80" />
            <span>© {new Date().getFullYear()} Askolo</span>
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

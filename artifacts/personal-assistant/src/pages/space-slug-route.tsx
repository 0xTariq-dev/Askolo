import { useEffect, useState } from 'react';
import { Link, useLocation, useRoute } from 'wouter';
import { AlertCircle, LoaderCircle } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { useSpaceScope } from '@/contexts/space-context';

export function SpaceSlugRoute() {
  const [, params] = useRoute('/s/:slug');
  const slug = params?.slug?.toLowerCase() ?? '';
  const { spaces, loading, error, setSelectedSpaceId } = useSpaceScope();
  const [, setLocation] = useLocation();
  const [notFound, setNotFound] = useState(false);

  useEffect(() => {
    if (loading || error || !slug) return;
    const space = spaces.find((candidate) => candidate.slug?.toLowerCase() === slug);
    if (!space) {
      setNotFound(true);
      return;
    }
    setSelectedSpaceId(space.id);
    setLocation('/home');
  }, [error, loading, setLocation, setSelectedSpaceId, slug, spaces]);

  if (loading) {
    return (
      <main className="grid min-h-[50vh] place-items-center p-6">
        <div role="status" className="flex items-center gap-3 text-sm text-muted-foreground">
          <LoaderCircle aria-hidden="true" className="size-4 animate-spin" />
          Checking Space access…
        </div>
      </main>
    );
  }

  if (error || notFound) {
    return (
      <main className="mx-auto grid min-h-[50vh] max-w-lg content-center gap-4 p-6">
        <div role="alert" className="flex items-start gap-3 rounded-xl border border-border bg-card p-5">
          <AlertCircle aria-hidden="true" className="mt-0.5 size-5 shrink-0 text-destructive" />
          <div>
            <h1 className="font-display text-lg font-semibold">
              {error ? 'Spaces could not be loaded' : 'Space not found'}
            </h1>
            <p className="mt-1 text-sm text-muted-foreground">
              {error ?? 'This link may be outdated, or your account may not have access to that Space.'}
            </p>
          </div>
        </div>
        <Button asChild variant="outline" className="w-fit">
          <Link href="/home">Go to Home</Link>
        </Button>
      </main>
    );
  }

  return (
    <main className="grid min-h-[50vh] place-items-center p-6">
      <div role="status" className="flex items-center gap-3 text-sm text-muted-foreground">
        <LoaderCircle aria-hidden="true" className="size-4 animate-spin" />
        Opening Space…
      </div>
    </main>
  );
}

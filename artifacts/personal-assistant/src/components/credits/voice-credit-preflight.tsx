import { useCallback, useEffect, useState } from 'react';
import { Link } from 'wouter';
import { Coins, RefreshCw } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { creditApi, creditErrorMessage, formatUsdMicros, type CreditEstimate } from '@/lib/credit-api';

export function VoiceCreditPreflight({ showWalletLink = true }: { showWalletLink?: boolean }) {
  const [estimate, setEstimate] = useState<CreditEstimate | null>(null);
  const [error, setError] = useState('');
  const [loading, setLoading] = useState(true);

  const loadEstimate = useCallback(async () => {
    setLoading(true);
    setError('');
    try {
      setEstimate(await creditApi.estimate('voice.recorded', 60));
    } catch (loadError) {
      setError(creditErrorMessage(loadError, 'The voice estimate is unavailable.'));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void loadEstimate();
  }, [loadEstimate]);

  return (
    <Card aria-labelledby="voice-credit-preflight-title" className="mb-4 border-primary/20 bg-primary/[0.04]">
      <CardHeader className="flex flex-col gap-3 pb-2 sm:flex-row sm:items-center sm:justify-between">
        <div>
          <CardTitle id="voice-credit-preflight-title" className="flex items-center gap-2 text-sm">
            <Coins className="h-4 w-4 text-primary" />
            Voice credit estimate
          </CardTitle>
          <CardDescription className="mt-1">
            Check the current estimate before recording or starting live transcription.
          </CardDescription>
        </div>
        <div className="flex flex-wrap gap-2">
          <Button type="button" variant="outline" size="sm" onClick={() => void loadEstimate()} disabled={loading} aria-label="Refresh voice credit estimate">
            <RefreshCw className={`mr-2 h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} />
            Refresh
          </Button>
          {showWalletLink && (
            <Link href="/credits" className="inline-flex min-h-9 items-center rounded-md border border-border px-3 text-sm font-medium transition-colors hover:bg-accent focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
              View credits
            </Link>
          )}
        </div>
      </CardHeader>
      <CardContent className="pt-1" aria-live="polite" aria-atomic="true">
        {loading && <p role="status" className="text-sm text-muted-foreground">Loading the current estimate…</p>}
        {error && (
          <div className="flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between">
            <p role="alert" className="text-sm text-destructive">{error}</p>
            <Button type="button" variant="outline" size="sm" onClick={() => void loadEstimate()}>Try again</Button>
          </div>
        )}
        {estimate && !loading && !error && (
          <div className="flex flex-col gap-1 text-sm sm:flex-row sm:flex-wrap sm:items-center sm:gap-x-5">
            <p>
              <span className="text-muted-foreground">Estimated:</span>{' '}
             <strong><bdi>{formatUsdMicros(estimate.estimatedUsdMicros)}</bdi></strong>
            </p>
            <p>
              <span className="text-muted-foreground">Maximum reserved:</span>{' '}
               <strong><bdi>{formatUsdMicros(estimate.hardCapUsdMicros)}</bdi></strong>
            </p>
            <p>
              <span className="text-muted-foreground">Available:</span>{' '}
               <strong><bdi>{formatUsdMicros(estimate.availableUsdMicros)}</bdi></strong>
            </p>
            {!estimate.canReserve && (
              <p role="alert" className="basis-full font-medium text-destructive">
                 Your USD balance is below the amount needed to start voice input.
              </p>
            )}
            <p className="basis-full text-xs text-muted-foreground">
              Policy v{estimate.policyVersion}. The estimate is checked again when the request starts.
            </p>
          </div>
        )}
      </CardContent>
    </Card>
  );
}
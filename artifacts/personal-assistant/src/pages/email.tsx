import { PageTransition } from '@/components/ui/page-transition';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Mail, ExternalLink } from 'lucide-react';
import { useGetGoogleStatus } from '@workspace/api-client-react';

export function EmailPage() {
  const { data: googleStatus } = useGetGoogleStatus();

  return (
    <PageTransition className="max-w-5xl mx-auto pb-10">
      <header className="mb-8">
        <h1 className="text-3xl md:text-4xl font-display font-bold tracking-tight">Email Triage</h1>
        <p className="text-muted-foreground mt-2 text-lg">AI-powered inbox prioritization and reply drafting.</p>
      </header>

      <Card className="border-border bg-card/50 backdrop-blur-sm">
        <CardHeader>
          <CardTitle className="flex items-center gap-2 text-lg">
            <Mail className="h-5 w-5 text-primary" />
            Gmail not connected
          </CardTitle>
        </CardHeader>
        <CardContent className="space-y-4">
          <p className="text-muted-foreground">
            Email triage requires access to Gmail. Connect your Gmail account via Replit integrations to enable AI priority
            grouping and reply drafting.
          </p>
          <p className="text-sm text-muted-foreground">
            Currently connected to Google services:{' '}
            <span className={googleStatus?.connected ? 'text-emerald-400' : 'text-rose-400'}>
              {googleStatus?.connected ? 'Calendar' : 'None'}
            </span>
          </p>
          <Button variant="outline" className="gap-2" asChild>
            <a href="https://console.replit.com/integrations" target="_blank" rel="noreferrer">
              Open Integrations <ExternalLink className="h-4 w-4" />
            </a>
          </Button>
        </CardContent>
      </Card>
    </PageTransition>
  );
}

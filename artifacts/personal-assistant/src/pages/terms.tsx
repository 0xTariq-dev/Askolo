import { PageTransition } from '@/components/ui/page-transition';
import { Card, CardContent } from '@/components/ui/card';
import { PublicLayout } from '@/components/layout/public-layout';

export function TermsPage() {
  return (
    <PublicLayout>
      <PageTransition className="max-w-3xl mx-auto pb-10">
        <header className="mb-8">
        <h1 className="text-3xl md:text-4xl font-display font-bold tracking-tight">Terms of Service</h1>
        <p className="text-muted-foreground mt-2">Last updated: {new Date().toLocaleDateString()}</p>
      </header>

      <Card className="border-border bg-card/50 backdrop-blur-sm">
        <CardContent className="p-6 md:p-8 space-y-6 text-sm leading-relaxed text-foreground/90">
          <section>
            <h2 className="text-lg font-semibold mb-2">1. Acceptance of terms</h2>
            <p>
              By accessing or using Askolo, you agree to be bound by these Terms of Service. If you do not agree to these
              terms, please do not use the service.
            </p>
          </section>

          <section>
            <h2 className="text-lg font-semibold mb-2">2. Description of service</h2>
            <p>
              Askolo is an AI-powered personal assistant that helps you manage habits, goals, daily plans, notes, action
              items, chores, calendar events, and email. Optional integrations with Google Calendar and Gmail are available
              with your explicit consent.
            </p>
          </section>

          <section>
            <h2 className="text-lg font-semibold mb-2">3. User accounts and responsibilities</h2>
            <p>
              You are responsible for maintaining the confidentiality of your account and for all activities that occur under
              your account. You agree to use the service in compliance with applicable laws and not to misuse the AI features or
              integrations.
            </p>
          </section>

          <section>
            <h2 className="text-lg font-semibold mb-2">4. AI-generated content</h2>
            <p>
              Askolo uses AI to generate suggestions, summaries, and drafts. AI-generated content is provided for your
              convenience and should be reviewed before you rely on it or act on it. We are not responsible for decisions
              made based on AI-generated suggestions.
            </p>
          </section>

          <section>
            <h2 className="text-lg font-semibold mb-2">5. Intellectual property</h2>
            <p>
              All content, features, and functionality of Askolo are owned by Askolo and its licensors. You retain ownership
              of the data you create in the app.
            </p>
          </section>

          <section>
            <h2 className="text-lg font-semibold mb-2">6. Termination</h2>
            <p>
              We reserve the right to suspend or terminate your access to the service at any time for violations of these
              terms or for any other reason.
            </p>
          </section>

          <section>
            <h2 className="text-lg font-semibold mb-2">7. Changes to terms</h2>
            <p>
              We may update these Terms of Service from time to time. Continued use of the service after changes constitutes
              acceptance of the revised terms.
            </p>
          </section>

          <section>
            <h2 className="text-lg font-semibold mb-2">8. Contact</h2>
            <p>If you have questions about these Terms of Service, please contact the Askolo team.</p>
          </section>
        </CardContent>
        </Card>
      </PageTransition>
    </PublicLayout>
  );
}

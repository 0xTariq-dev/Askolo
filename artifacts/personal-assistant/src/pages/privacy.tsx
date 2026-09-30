import { useEffect } from 'react';
import { PageTransition } from '@/components/ui/page-transition';
import { Card, CardContent } from '@/components/ui/card';
import { PublicLayout } from '@/components/layout/public-layout';
import { setPageMetadata } from '@/lib/seo';

export function PrivacyPage() {
  useEffect(() => {
    setPageMetadata({
      title: 'Askolo Privacy Policy | Data Protection and Your Rights',
      description:
        'Read the Askolo Privacy Policy to understand how we collect, use, store, and protect your personal data and Google integration data.',
      canonicalPath: '/privacy',
    });
  }, []);

  return (
    <PublicLayout>
      <PageTransition className="max-w-3xl mx-auto pb-10">
        <header className="mb-8">
        <h1 className="text-3xl md:text-4xl font-display font-bold tracking-tight">Privacy Policy</h1>
        <p className="text-muted-foreground mt-2">Last updated: {new Date().toLocaleDateString()}</p>
      </header>

      <Card className="border-border bg-card/50 backdrop-blur-sm">
        <CardContent className="p-6 md:p-8 space-y-6 text-sm leading-relaxed text-foreground/90">
          <section>
            <h2 className="text-lg font-semibold mb-2">1. Information we collect</h2>
            <p>
              Askolo collects information you provide directly, such as your name, email address, and the content you
              create in the app (tasks, habits, goals, notes, calendar events, and chores). With your permission, we also
              access selected Google data (Calendar events and Gmail metadata) through official OAuth integrations.
            </p>
          </section>

          <section>
            <h2 className="text-lg font-semibold mb-2">2. How we use your information</h2>
            <p>
              We use your information to provide the Askolo service, personalize your experience, generate AI-powered
              suggestions, and synchronize your calendar and email data when you have connected those integrations. We do not
              sell your personal information to third parties.
            </p>
          </section>

          <section>
            <h2 className="text-lg font-semibold mb-2">3. Data storage and security</h2>
            <p>
              Your data is stored securely in our infrastructure. We use industry-standard encryption for data in transit and
              at rest. Access to Google services is handled through OAuth tokens that are refreshed and stored securely by Askolo
              using your own Google Cloud OAuth app.
            </p>
          </section>

          <section>
            <h2 className="text-lg font-semibold mb-2">4. Third-party services</h2>
            <p>
              Askolo integrates with Google Calendar and Gmail. These integrations are optional and governed by Google's
              privacy policy and terms of service. You can revoke access at any time through your Google account settings.
            </p>
          </section>

          <section>
            <h2 className="text-lg font-semibold mb-2">5. Your rights</h2>
            <p>
              You can access, update, or delete your data within the app. If you wish to delete your account entirely,
              please contact us.
            </p>
          </section>

          <section>
            <h2 className="text-lg font-semibold mb-2">6. Contact</h2>
            <p>If you have questions about this Privacy Policy, please contact the Askolo team.</p>
          </section>
        </CardContent>
        </Card>
      </PageTransition>
    </PublicLayout>
  );
}

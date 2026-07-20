import { Link } from 'wouter';
import { useAuth } from '@clerk/react';
import { motion } from 'framer-motion';
import {
  CheckCircle2,
  Target,
  ListTodo,
  Calendar,
  ClipboardList,
  StickyNote,
  Zap,
  Mail,
  Sparkles,
  ShieldCheck,
  ArrowRight,
} from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { PublicLayout } from '@/components/layout/public-layout';
import logoUrl from '/logo.png';

const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');

const features = [
  {
    icon: CheckCircle2,
    title: 'Habits',
    description: 'Build streaks and track daily routines that stick.',
  },
  {
    icon: Target,
    title: 'Goals',
    description: 'Set long-term goals and break them into actionable steps.',
  },
  {
    icon: ListTodo,
    title: 'Daily Plan',
    description: 'Plan your day with focused time blocks and priorities.',
  },
  {
    icon: Calendar,
    title: 'Calendar',
    description: 'See local events and synced Google Calendar in one view.',
  },
  {
    icon: ClipboardList,
    title: 'Chores',
    description: 'Keep household tasks organized and recurring.',
  },
  {
    icon: StickyNote,
    title: 'Notes',
    description: 'Capture thoughts, ideas, and reference material.',
  },
  {
    icon: Zap,
    title: 'Actions',
    description: 'Track action items and deadlines across your life.',
  },
  {
    icon: Sparkles,
    title: 'AI Coach',
    description: 'Get personalized insights and suggestions from your AI assistant.',
  },
];

export function LandingPage() {
  const { isSignedIn } = useAuth();

  const ctaHref = isSignedIn ? '/dashboard' : '/sign-up';
  const ctaLabel = isSignedIn ? 'Go to Dashboard' : 'Get Started Free';

  return (
    <PublicLayout>
      {/* Hero */}
      <section className="pt-16 sm:pt-24 pb-16 px-4 sm:px-6">
        <div className="max-w-4xl mx-auto text-center">
          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.8, ease: 'easeOut' }}
            className="flex flex-col items-center"
          >
            <div className="h-24 w-auto mb-8 relative">
              <img src={logoUrl} alt="Askolo" className="h-full w-auto object-contain drop-shadow-2xl" />
            </div>
            <h1 className="text-4xl sm:text-5xl md:text-6xl font-display font-bold tracking-tight mb-6">
              Meet <span className="text-primary">Askolo</span>
            </h1>
            <p className="text-lg sm:text-xl text-muted-foreground max-w-2xl mx-auto mb-10">
              Your beautifully designed mission control for habits, goals, daily plans, and family life. Askolo brings
              everything together in one calm, focused place.
            </p>
            <div className="flex flex-col sm:flex-row gap-4">
              <Button
                size="lg"
                asChild
                className="rounded-full px-8 py-6 text-lg font-medium shadow-[0_0_40px_-10px_rgba(234,179,8,0.3)] hover:shadow-[0_0_60px_-10px_rgba(234,179,8,0.5)] transition-all duration-300 group"
              >
                <Link href={ctaHref}>
                  {ctaLabel}
                  <ArrowRight className="ml-2 h-5 w-5 group-hover:translate-x-1 transition-transform" />
                </Link>
              </Button>
              <Button size="lg" variant="outline" asChild className="rounded-full px-8 py-6 text-lg font-medium border-white/10 hover:bg-white/5 transition-all">
                <Link href={isSignedIn ? '/dashboard' : '/sign-in'}>
                  {isSignedIn ? 'Open Dashboard' : 'Sign In'}
                </Link>
              </Button>
            </div>
          </motion.div>
        </div>
      </section>

      {/* Features */}
      <section className="py-16 px-4 sm:px-6 bg-white/[0.02] border-y border-border/40">
        <div className="max-w-6xl mx-auto">
          <div className="text-center mb-12">
            <h2 className="text-2xl sm:text-3xl font-display font-bold mb-3">Everything you need to stay on track</h2>
            <p className="text-muted-foreground max-w-xl mx-auto">
              Askolo combines personal productivity tools with an AI assistant so you can focus on what matters.
            </p>
          </div>
          <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
            {features.map((feature, index) => (
              <motion.div
                key={feature.title}
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.5, delay: index * 0.05 }}
              >
                <Card className="h-full bg-card/40 border-border hover:bg-card/60 transition-colors">
                  <CardContent className="p-5">
                    <div className="h-10 w-10 rounded-xl bg-primary/10 border border-primary/20 flex items-center justify-center mb-4">
                      <feature.icon className="h-5 w-5 text-primary" />
                    </div>
                    <h3 className="font-display font-semibold mb-1">{feature.title}</h3>
                    <p className="text-sm text-muted-foreground">{feature.description}</p>
                  </CardContent>
                </Card>
              </motion.div>
            ))}
          </div>
        </div>
      </section>

      {/* Google integration explanation */}
      <section className="py-16 px-4 sm:px-6">
        <div className="max-w-3xl mx-auto">
          <div className="text-center mb-10">
            <h2 className="text-2xl sm:text-3xl font-display font-bold mb-3">Optional Google integrations</h2>
            <p className="text-muted-foreground">
              Askolo can connect to your Google account to keep your calendar and email in sync. These integrations are
              completely optional, and you are in control of your data.
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
            <Card className="bg-card/40 border-border">
              <CardContent className="p-6">
                <div className="flex items-center gap-3 mb-3">
                  <div className="h-10 w-10 rounded-xl bg-blue-500/10 border border-blue-500/20 flex items-center justify-center">
                    <Calendar className="h-5 w-5 text-blue-400" />
                  </div>
                  <h3 className="font-display font-semibold">Google Calendar</h3>
                </div>
                <p className="text-sm text-muted-foreground mb-4">
                  Askolo asks for Calendar access so you can:
                </p>
                <ul className="text-sm text-muted-foreground space-y-2 list-disc pl-4">
                  <li>Sync your Google events into your Askolo calendar</li>
                  <li>Create new events from Askolo and save them to Google</li>
                  <li>Edit or delete events in either place and keep them in sync</li>
                </ul>
              </CardContent>
            </Card>

            <Card className="bg-card/40 border-border">
              <CardContent className="p-6">
                <div className="flex items-center gap-3 mb-3">
                  <div className="h-10 w-10 rounded-xl bg-red-500/10 border border-red-500/20 flex items-center justify-center">
                    <Mail className="h-5 w-5 text-red-400" />
                  </div>
                  <h3 className="font-display font-semibold">Gmail</h3>
                </div>
                <p className="text-sm text-muted-foreground mb-4">
                  Askolo asks for Gmail access so you can:
                </p>
                <ul className="text-sm text-muted-foreground space-y-2 list-disc pl-4">
                  <li>Triage your inbox with AI-powered priority labels</li>
                  <li>Draft replies in your chosen tone with one click</li>
                  <li>Send messages directly from Askolo</li>
                </ul>
              </CardContent>
            </Card>
          </div>

          <div className="mt-8 flex items-start gap-3 rounded-xl border border-border/60 bg-card/30 p-4 text-sm text-muted-foreground">
            <ShieldCheck className="h-5 w-5 text-emerald-400 shrink-0 mt-0.5" />
            <p>
              The consent screen shows <strong>Askolo</strong> because the app uses your own Google Cloud OAuth app. You
              can revoke access at any time in your{' '}
              <a href="https://myaccount.google.com/permissions" target="_blank" rel="noopener noreferrer" className="text-primary hover:underline">
                Google account settings
              </a>.
            </p>
          </div>
        </div>
      </section>

      {/* Final CTA */}
      <section className="py-16 px-4 sm:px-6 text-center">
        <div className="max-w-2xl mx-auto">
          <h2 className="text-2xl sm:text-3xl font-display font-bold mb-4">Ready to get organized?</h2>
          <p className="text-muted-foreground mb-8">
            Join Askolo and start building better habits, clearer goals, and calmer days.
          </p>
          <Button size="lg" asChild className="rounded-full px-8 py-6 text-lg font-medium shadow-[0_0_40px_-10px_rgba(234,179,8,0.3)] hover:shadow-[0_0_60px_-10px_rgba(234,179,8,0.5)] transition-all duration-300 group">
            <Link href={ctaHref}>
              {ctaLabel}
              <ArrowRight className="ml-2 h-5 w-5 group-hover:translate-x-1 transition-transform" />
            </Link>
          </Button>
        </div>
      </section>
    </PublicLayout>
  );
}

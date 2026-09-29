import { useEffect, useState } from 'react';
import { LayoutGroup, motion, useReducedMotion } from 'framer-motion';
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
  Star,
} from 'lucide-react';
import { Button } from '@workspace/askolo-design-system/components/ui/button';
import { Card, CardContent } from '@workspace/askolo-design-system/components/ui/card';
import { AskoloTunnelConcept } from '@workspace/askolo-design-system/components/hero-animation/AskoloTunnelConcept';
import { useAskoloTheme } from '@workspace/askolo-design-system/theme';
import { PublicLayout } from '@/components/layout/public-layout';
import { AnimatedBrandName } from '@/components/animated-brand-name';
import { BrandSlogan, BrandSloganFormation } from '@/components/landing/brand-slogan';
import logoUrl from '/logo.png';
import { toAppUrl } from '@/lib/site-domains';
import { setPageMetadata } from '@/lib/seo';

const features = [
  {
    icon: CheckCircle2,
    title: 'Habits',
    description: 'Build streaks and track daily routines that stick with simple check-ins.',
  },
  {
    icon: Target,
    title: 'Goals',
    description: 'Set long-term goals, add milestones, and watch your progress over time.',
  },
  {
    icon: ListTodo,
    title: 'Daily Plan',
    description: 'Plan each day with focused time blocks and clear priorities.',
  },
  {
    icon: Calendar,
    title: 'Calendar',
    description: 'See local events and synced Google Calendar in one clean view.',
  },
  {
    icon: ClipboardList,
    title: 'Chores',
    description: 'Keep household tasks organized, recurring, and fairly assigned.',
  },
  {
    icon: StickyNote,
    title: 'Notes',
    description: 'Capture thoughts, ideas, and reference material in one place.',
  },
  {
    icon: Zap,
    title: 'Actions',
    description: 'Track action items, deadlines, and follow-ups across your life.',
  },
  {
    icon: Sparkles,
    title: 'AI Coach',
    description: 'Get personalized insights and suggestions from your built-in assistant.',
  },
];

const howItWorks = [
  {
    step: '1',
    title: 'Set your priorities',
    description: 'Add habits, goals, and daily plans that reflect what matters most to you.',
  },
  {
    step: '2',
    title: 'Connect your calendar',
    description: 'Optionally link Google Calendar and Gmail so everything stays in sync.',
  },
  {
    step: '3',
    title: 'Askolo keeps you on track',
    description: 'Your AI assistant surfaces reminders, drafts replies, and helps you focus.',
  },
];

export function LandingPage() {
  const prefersReducedMotion = useReducedMotion();
  const [brandFormed, setBrandFormed] = useState(Boolean(prefersReducedMotion));
  const {
    preferences: { mode },
  } = useAskoloTheme();
  const ctaHref = toAppUrl('/sign-up', { theme: mode });
  const secondaryHref = toAppUrl('/sign-in', { theme: mode });
  const pageBackground = (
    <div
      aria-hidden="true"
      className="pointer-events-none sticky top-0 z-0 -mb-[100dvh] h-dvh w-full shrink-0 overflow-hidden opacity-40"
    >
      <AskoloTunnelConcept
        theme={mode}
        reducedMotion={Boolean(prefersReducedMotion)}
      />
    </div>
  );

  useEffect(() => {
    setPageMetadata({
      title: 'AI Personal & Family Assistant for Calmer Days | Askolo',
      description:
        'Askolo is a private AI personal and family assistant for habits, goals, daily plans, calendar, chores, notes, and email.',
    });
  }, []);

  useEffect(() => {
    if (prefersReducedMotion) {
      setBrandFormed(true);
      return;
    }

    setBrandFormed(false);
    const timeout = window.setTimeout(() => setBrandFormed(true), 1250);
    return () => window.clearTimeout(timeout);
  }, [prefersReducedMotion]);

  return (
    <PublicLayout background={pageBackground}>
      {/* Hero */}
      <section
        id="hero"
        aria-labelledby="landing-heading"
        className="relative"
      >
        <div className="relative z-20 mx-auto max-w-6xl px-4 pb-16 pt-16 text-center sm:px-6 sm:pb-24 sm:pt-24">
          <LayoutGroup id="askolo-landing-brand">
            <div className="sticky top-20 z-30 mx-auto mb-8 flex w-fit max-w-full flex-col items-center">
              <motion.div
                initial={prefersReducedMotion ? false : { opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={prefersReducedMotion ? { duration: 0 } : { duration: 0.7, delay: 0.1, ease: 'easeOut' }}
                className="mb-5 h-40 w-auto sm:h-44"
              >
                <img
                  src={logoUrl}
                  alt=""
                  width={64}
                  height={64}
                  className="h-full w-auto object-contain drop-shadow-2xl"
                />
              </motion.div>

              <h1
                id="landing-heading"
                aria-label="Askolo"
                className="min-h-[1.1em] max-w-full text-4xl font-display font-bold tracking-tight sm:text-5xl md:text-6xl lg:text-7xl"
              >
                {brandFormed ? (
                  <AnimatedBrandName layoutIdPrefix="askolo-landing" />
                ) : (
                  <span aria-hidden="true" className="invisible inline-block">
                    Askolo
                  </span>
                )}
              </h1>
            </div>

            <div className="mx-auto mb-10 max-w-5xl px-2">
              {brandFormed ? (
                <BrandSlogan />
              ) : (
                <BrandSloganFormation
                  layoutIdPrefix="askolo-landing"
                  reduceMotion={Boolean(prefersReducedMotion)}
                />
              )}
            </div>
          </LayoutGroup>

          <motion.div
            initial={prefersReducedMotion ? false : { opacity: 0, y: 20 }}
            animate={{ opacity: brandFormed ? 1 : 0, y: brandFormed ? 0 : 12 }}
            transition={prefersReducedMotion ? { duration: 0 } : { duration: 0.55, ease: 'easeOut' }}
            aria-hidden={!brandFormed}
            className="mx-auto mb-10 max-w-3xl"
          >
            <p className="text-sm leading-relaxed text-muted-foreground sm:text-base">
              Askolo brings habits, goals, daily plans, calendar, chores, notes,
              and action items into one calm workspace — with an AI coach that
              helps individuals and families stay on track.
            </p>
          </motion.div>

          <motion.div
            initial={prefersReducedMotion ? false : { opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={prefersReducedMotion ? { duration: 0 } : { duration: 0.7, delay: 0.4, ease: 'easeOut' }}
            className="flex flex-col sm:flex-row gap-4 justify-center"
          >
            <Button
              size="default"
              asChild
              className="group min-h-11 rounded-full"
            >
              <a href={ctaHref}>
                Get Started Free
                <ArrowRight className="ml-2 h-5 w-5 group-hover:translate-x-1 transition-transform" />
              </a>
            </Button>
            <Button
              size="default"
              variant="outline"
              asChild
              className="min-h-11 rounded-full"
            >
              <a href={secondaryHref}>Sign In</a>
            </Button>
          </motion.div>

          <motion.div
            initial={prefersReducedMotion ? false : { opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={prefersReducedMotion ? { duration: 0 } : { duration: 0.7, delay: 0.6, ease: 'easeOut' }}
            className="mt-10 flex flex-wrap items-center justify-center gap-6 text-xs text-muted-foreground"
          >
            <span className="flex items-center gap-1.5">
              <Star className="h-3.5 w-3.5 text-primary" /> Built for individuals and families
            </span>
          </motion.div>
        </div>
      </section>

      {/* Features */}
      <section
        id="features"
        aria-labelledby="features-heading"
        className="scroll-mt-20 py-16 sm:py-24 px-4 sm:px-6"
      >
        <div className="max-w-6xl mx-auto">
          <div className="text-center mb-12 sm:mb-16">
            <h2 id="features-heading" className="text-2xl sm:text-3xl md:text-4xl font-display font-bold mb-4">
              Everything you need to stay organized
            </h2>
            <p className="text-muted-foreground max-w-2xl mx-auto text-base sm:text-lg">
              Askolo combines personal productivity tools with an AI assistant so you can focus on what matters most.
            </p>
          </div>

          <ul className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 list-none p-0 m-0">
            {features.map((feature, index) => (
              <motion.li
                key={feature.title}
                initial={prefersReducedMotion ? false : { opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={prefersReducedMotion ? { duration: 0 } : { duration: 0.5, delay: index * 0.05 }}
              >
                <Card className="h-full bg-card/40 border-border hover:bg-card/60 hover:border-primary/20 transition-all duration-300">
                  <CardContent className="p-5">
                    <div className="h-10 w-10 rounded-xl bg-primary/10 border border-primary/20 flex items-center justify-center mb-4">
                      <feature.icon className="h-5 w-5 text-primary" />
                    </div>
                    <h3 className="font-display font-semibold mb-1.5">{feature.title}</h3>
                    <p className="text-sm text-muted-foreground leading-relaxed">{feature.description}</p>
                  </CardContent>
                </Card>
              </motion.li>
            ))}
          </ul>
        </div>
      </section>

      {/* How it works */}
      <section
        id="how-it-works"
        aria-labelledby="how-it-works-heading"
        className="scroll-mt-20 py-16 sm:py-24 px-4 sm:px-6"
      >
        <div className="max-w-5xl mx-auto">
          <div className="text-center mb-12 sm:mb-16">
            <h2 id="how-it-works-heading" className="text-2xl sm:text-3xl md:text-4xl font-display font-bold mb-4">
              How Askolo works
            </h2>
            <p className="text-muted-foreground max-w-2xl mx-auto text-base sm:text-lg">
              A simple workflow that keeps your life organized without overwhelming you.
            </p>
          </div>

          <ol className="grid grid-cols-1 md:grid-cols-3 gap-6 list-none p-0 m-0">
            {howItWorks.map((item, index) => (
              <motion.li
                key={item.step}
                initial={prefersReducedMotion ? false : { opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={prefersReducedMotion ? { duration: 0 } : { duration: 0.5, delay: 0.2 + index * 0.1 }}
              >
                <Card className="h-full bg-card/40 border-border">
                  <CardContent className="p-6">
                    <div className="h-10 w-10 rounded-full bg-primary/10 border border-primary/20 flex items-center justify-center mb-4">
                      <span className="text-sm font-bold text-primary">{item.step}</span>
                    </div>
                    <h3 className="font-display font-semibold text-lg mb-2">{item.title}</h3>
                    <p className="text-sm text-muted-foreground leading-relaxed">{item.description}</p>
                  </CardContent>
                </Card>
              </motion.li>
            ))}
          </ol>
        </div>
      </section>

      {/* Google integration explanation */}
      <section
        id="integrations"
        aria-labelledby="integrations-heading"
        className="scroll-mt-20 py-16 sm:py-24 px-4 sm:px-6"
      >
        <div className="max-w-5xl mx-auto">
          <div className="text-center mb-10 sm:mb-14">
            <h2 id="integrations-heading" className="text-2xl sm:text-3xl md:text-4xl font-display font-bold mb-4">
              Optional Google integrations
            </h2>
            <p className="text-muted-foreground max-w-2xl mx-auto text-base sm:text-lg">
              Askolo can connect to your Google account to keep your calendar and email in sync. These integrations are completely optional, and you are always in control of your data.
            </p>
          </div>

          <div className="grid grid-cols-1 md:grid-cols-2 gap-5">
            <Card className="bg-card/40 border-border">
              <CardContent className="p-6 sm:p-8">
                <div className="flex items-center gap-3 mb-4">
                  <div className="h-12 w-12 rounded-xl bg-info/10 border border-info/20 flex items-center justify-center">
                    <Calendar className="h-6 w-6 text-info" />
                  </div>
                  <h3 className="font-display font-semibold text-lg">Google Calendar</h3>
                </div>
                <p className="text-sm text-muted-foreground mb-4 leading-relaxed">
                  Askolo asks for Calendar access so you can:
                </p>
                <ul className="text-sm text-muted-foreground space-y-3 list-disc pl-5 leading-relaxed">
                  <li>Sync your Google events into your Askolo calendar</li>
                  <li>Create new events from Askolo and save them to Google</li>
                  <li>Edit or delete events in either place and keep them in sync</li>
                </ul>
              </CardContent>
            </Card>

            <Card className="bg-card/40 border-border">
              <CardContent className="p-6 sm:p-8">
                <div className="flex items-center gap-3 mb-4">
                  <div className="h-12 w-12 rounded-xl bg-destructive/10 border border-destructive/20 flex items-center justify-center">
                    <Mail className="h-6 w-6 text-destructive" />
                  </div>
                  <h3 className="font-display font-semibold text-lg">Gmail</h3>
                </div>
                <p className="text-sm text-muted-foreground mb-4 leading-relaxed">
                  Askolo asks for Gmail access so you can:
                </p>
                <ul className="text-sm text-muted-foreground space-y-3 list-disc pl-5 leading-relaxed">
                  <li>Triage your inbox with AI-powered priority labels</li>
                  <li>Draft replies in your chosen tone with one click</li>
                  <li>Send messages directly from Askolo</li>
                </ul>
              </CardContent>
            </Card>
          </div>

          <div className="mt-8 flex items-start gap-4 rounded-xl border border-border/60 bg-card/30 p-5 text-sm text-muted-foreground">
            <ShieldCheck className="h-5 w-5 text-success shrink-0 mt-0.5" />
            <p className="leading-relaxed">
              The Google consent screen shows <strong>Askolo</strong> because the app uses its own Google Cloud OAuth project. You can revoke access at any time in your{' '}
              <a
                href="https://myaccount.google.com/permissions"
                target="_blank"
                rel="noopener noreferrer"
                className="text-primary hover:underline"
              >
                Google account settings
              </a>.
            </p>
          </div>
        </div>
      </section>

      {/* Final CTA */}
      <section
        id="final-cta"
        aria-labelledby="final-cta-heading"
        className="scroll-mt-20 py-16 sm:py-24 px-4 sm:px-6 text-center"
      >
        <div className="max-w-3xl mx-auto">
          <h2 id="final-cta-heading" className="text-2xl sm:text-3xl md:text-4xl font-display font-bold mb-4">
            Ready to get organized?
          </h2>
          <p className="text-muted-foreground text-base sm:text-lg mb-8 max-w-xl mx-auto">
            Join Askolo and start building better habits, clearer goals, and calmer days — all in one place.
          </p>
          <Button
            size="lg"
            asChild
            className="rounded-full px-8 py-6 text-lg font-medium shadow-lg transition-all duration-300 group"
          >
            <a href={ctaHref}>
              Get Started Free
              <ArrowRight className="ml-2 h-5 w-5 group-hover:translate-x-1 transition-transform" />
            </a>
          </Button>
        </div>
      </section>
    </PublicLayout>
  );
}

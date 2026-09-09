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
  ArrowUpRight,
  Play,
  Star,
  Users,
  Lock,
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
  const { isSignedIn } = useAuth();

  const ctaHref = isSignedIn ? '/dashboard' : '/sign-up';
  const ctaLabel = isSignedIn ? 'Go to Dashboard' : 'Get Started Free';
  const secondaryHref = isSignedIn ? '/dashboard' : '/sign-in';
  const secondaryLabel = isSignedIn ? 'Open Dashboard' : 'Sign In';

  return (
    <PublicLayout>
      {/* Hero */}
      <section id="hero" aria-labelledby="landing-heading" className="relative overflow-hidden">
        <div className="absolute inset-0 bg-gradient-to-b from-primary/5 via-transparent to-transparent pointer-events-none" />
        <div className="relative max-w-6xl mx-auto px-4 sm:px-6 pt-16 sm:pt-24 pb-16 sm:pb-24 text-center">
          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.7, ease: 'easeOut' }}
          >
            <div className="inline-flex items-center gap-2 px-3 py-1 rounded-full bg-primary/10 border border-primary/20 text-primary text-xs font-medium mb-8">
              <Sparkles className="h-3.5 w-3.5" />
              Your personal and family assistant
            </div>
          </motion.div>

          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.7, delay: 0.1, ease: 'easeOut' }}
          >
            <div className="h-24 sm:h-28 w-auto mb-8 mx-auto relative">
              <img
                src={logoUrl}
                alt="Askolo"
                width={64}
                height={64}
                className="h-full w-auto object-contain drop-shadow-2xl mx-auto"
              />
            </div>
          </motion.div>

          <motion.h1
            id="landing-heading"
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.7, delay: 0.2, ease: 'easeOut' }}
            className="text-4xl sm:text-5xl md:text-6xl lg:text-7xl font-display font-bold tracking-tight mb-6"
          >
            Your AI personal assistant for <span className="text-primary">calmer days</span>
          </motion.h1>

          <motion.p
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.7, delay: 0.3, ease: 'easeOut' }}
            className="text-lg sm:text-xl text-muted-foreground max-w-2xl mx-auto mb-10 leading-relaxed"
          >
            Askolo brings habits, goals, daily plans, calendar, chores, notes, and action items into one calm workspace — with an AI coach that helps individuals and families stay on track.
          </motion.p>

          <motion.div
            initial={{ opacity: 0, y: 20 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{ duration: 0.7, delay: 0.4, ease: 'easeOut' }}
            className="flex flex-col sm:flex-row gap-4 justify-center"
          >
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
            <Button
              size="lg"
              variant="outline"
              asChild
              className="rounded-full px-8 py-6 text-lg font-medium border-white/10 hover:bg-white/5 transition-all"
            >
              <Link href={secondaryHref}>
                {secondaryLabel}
              </Link>
            </Button>
          </motion.div>

          <motion.div
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            transition={{ duration: 0.7, delay: 0.6, ease: 'easeOut' }}
            className="mt-10 flex flex-wrap items-center justify-center gap-6 text-xs text-muted-foreground"
          >
            <span className="flex items-center gap-1.5">
              <Lock className="h-3.5 w-3.5 text-emerald-400" /> Private by default
            </span>
            <span className="flex items-center gap-1.5">
              <ShieldCheck className="h-3.5 w-3.5 text-emerald-400" /> Google integrations are optional
            </span>
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
        style={{ contentVisibility: 'auto', containIntrinsicSize: '0 720px' }}
        className="py-16 sm:py-24 px-4 sm:px-6 bg-white/[0.02] border-y border-border/40"
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
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.5, delay: index * 0.05 }}
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
        style={{ contentVisibility: 'auto', containIntrinsicSize: '0 560px' }}
        className="py-16 sm:py-24 px-4 sm:px-6"
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
                initial={{ opacity: 0, y: 20 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ duration: 0.5, delay: 0.2 + index * 0.1 }}
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
        style={{ contentVisibility: 'auto', containIntrinsicSize: '0 700px' }}
        className="py-16 sm:py-24 px-4 sm:px-6 bg-white/[0.02] border-y border-border/40"
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
                  <div className="h-12 w-12 rounded-xl bg-blue-500/10 border border-blue-500/20 flex items-center justify-center">
                    <Calendar className="h-6 w-6 text-blue-400" />
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
                  <div className="h-12 w-12 rounded-xl bg-red-500/10 border border-red-500/20 flex items-center justify-center">
                    <Mail className="h-6 w-6 text-red-400" />
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
            <ShieldCheck className="h-5 w-5 text-emerald-400 shrink-0 mt-0.5" />
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
        style={{ contentVisibility: 'auto', containIntrinsicSize: '0 360px' }}
        className="py-16 sm:py-24 px-4 sm:px-6 text-center"
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
            className="rounded-full px-8 py-6 text-lg font-medium shadow-[0_0_40px_-10px_rgba(234,179,8,0.3)] hover:shadow-[0_0_60px_-10px_rgba(234,179,8,0.5)] transition-all duration-300 group"
          >
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

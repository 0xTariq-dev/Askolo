import { useState, useEffect } from 'react';
import { motion } from 'framer-motion';
import { useGetDashboardSummary } from '@workspace/api-client-react';
import { 
  CheckCircle2, 
  Target, 
  ListTodo, 
  Calendar as CalendarIcon, 
  ClipboardList, 
  Zap,
  Flame,
  ArrowRight,
  Sparkles,
  Link2,
  Unlink,
  Mail
} from 'lucide-react';
import { Card, CardContent, CardHeader, CardTitle } from '@workspace/askolo-design-system/components/ui/card';
import { Button } from '@workspace/askolo-design-system/components/ui/button';
import { Link } from 'wouter';
import { isToday } from 'date-fns';
import { cn } from '@workspace/askolo-design-system/lib/utils';
import { goApi } from '@/lib/go-api';
import { PageTransition } from '@/components/ui/page-transition';
import { useLocale } from '@/contexts/locale-context';

export function DashboardPage() {
  const { formatDate, formatNumber } = useLocale();
  const { data: summary, isLoading } = useGetDashboardSummary();
  const [coaching, setCoaching] = useState<string | null>(null);
  const [coachingLoading, setCoachingLoading] = useState(true);

  useEffect(() => {
    if (!summary) return;
    const controller = new AbortController();
    let timeoutId: number | undefined;
    let idleId: number | undefined;
    let cancelled = false;

    const loadCoaching = () => {
      if (cancelled) return;

      goApi.coaching(
        {
          habits: summary.habits,
          goals: summary.goals,
          habitsCompletedToday: summary.habitsCompletedToday,
          habitsTotal: summary.habitsTotal,
        },
        controller.signal,
      )
        .then(data => { if (!cancelled && data.message) setCoaching(data.message); })
        .catch(() => {})
        .finally(() => {
          if (!cancelled) setCoachingLoading(false);
        });
    };

    const idleWindow = window as typeof window & {
      requestIdleCallback?: (callback: () => void, options?: { timeout: number }) => number;
      cancelIdleCallback?: (handle: number) => void;
    };

    if (idleWindow.requestIdleCallback) {
      idleId = idleWindow.requestIdleCallback(loadCoaching, { timeout: 2500 });
    } else {
      timeoutId = window.setTimeout(loadCoaching, 1200);
    }

    return () => {
      cancelled = true;
      controller.abort();
      if (idleId !== undefined) idleWindow.cancelIdleCallback?.(idleId);
      if (timeoutId !== undefined) window.clearTimeout(timeoutId);
    };
  }, [summary?.habitsCompletedToday, summary?.habitsTotal]);

  if (isLoading) {
    return (
      <div className="space-y-6">
        <div className="h-10 w-48 bg-muted rounded-md animate-pulse mb-8" />
        <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4">
          {[...Array(4)].map((_, i) => (
            <div key={i} className="h-32 bg-card rounded-xl animate-pulse" />
          ))}
        </div>
      </div>
    );
  }

  const google = summary?.googleConnection;
  const googleConnected = google?.calendarConnected || google?.gmailConnected;

  return (
    <PageTransition className="space-y-8 pb-10 max-w-7xl mx-auto">
      <header className="grid gap-6 rounded-2xl border border-border bg-card/70 p-6 shadow-sm md:grid-cols-[minmax(0,1fr)_auto] md:items-end md:p-8">
        <div>
          <p className="text-xs font-medium uppercase tracking-[0.2em] text-primary">Daily command center</p>
          <h1 className="mt-3 text-4xl font-display font-semibold tracking-tight md:text-5xl">Good morning.</h1>
          <p className="mt-3 max-w-xl text-base leading-relaxed text-muted-foreground">
            Here is your focus for {formatDate(new Date(), { weekday: 'long', month: 'long', day: 'numeric' })}. Small, clear steps are enough to move the day forward.
          </p>
        </div>
        <div className="rounded-xl border border-border bg-background/70 px-5 py-4 md:min-w-48">
          <p className="text-xs uppercase tracking-wider text-muted-foreground">Today’s progress</p>
          <p className="mt-1 text-3xl font-display font-semibold text-primary">
            {formatNumber(summary?.habitsCompletedToday || 0, { useGrouping: false })}<span className="text-lg text-muted-foreground">/{formatNumber(summary?.habitsTotal || 0, { useGrouping: false })}</span>
          </p>
          <p className="text-xs text-muted-foreground">habits completed</p>
        </div>
      </header>

      {/* Google status card */}
      <Card className={cn(
        'border-border bg-card/50 backdrop-blur-sm shadow-sm',
        googleConnected ? 'border-primary/30' : 'border-warning/30'
      )}>
        <CardContent className="p-4 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <div className={cn(
              'h-10 w-10 rounded-full flex items-center justify-center border',
              googleConnected ? 'bg-success/10 border-success/30 text-success' : 'bg-warning/10 border-warning/30 text-warning'
            )}>
              {googleConnected ? <Link2 className="h-5 w-5" /> : <Unlink className="h-5 w-5" />}
            </div>
            <div>
              <p className="font-medium">Google {googleConnected ? 'connected' : 'not connected'}</p>
              <p className="text-xs text-muted-foreground">
                {googleConnected
                  ? `${google?.calendarConnected ? 'Calendar synced' : ''}${google?.calendarConnected && google?.gmailConnected ? ' • ' : ''}${google?.gmailConnected ? 'Gmail ready' : ''}. Use the links below to open Calendar or Email triage.`
                  : 'Connect Google Calendar to sync events and enable Gmail for AI email triage.'}
              </p>
            </div>
          </div>
          <div className="flex items-center gap-2">
            <Link href="/calendar">
              <Button variant="outline" size="sm" className="border-border" disabled={!google?.calendarConnected}>
                <CalendarIcon className="h-4 w-4 mr-2" /> Calendar
              </Button>
            </Link>
            <Link href="/email">
              <Button variant="outline" size="sm" className="border-border" disabled={!google?.gmailConnected}>
                <Mail className="h-4 w-4 mr-2" /> Email
              </Button>
            </Link>
          </div>
        </CardContent>
      </Card>

      {/* Stats row */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <StatsCard 
          title="Habits Today" 
          value={`${summary?.habitsCompletedToday || 0}/${summary?.habitsTotal || 0}`}
          icon={CheckCircle2}
          color="text-success"
          bg="bg-success/10"
        />
        <StatsCard 
          title="Active Goals" 
          value={summary?.goalsActive || 0}
          icon={Target}
          color="text-warning"
          bg="bg-warning/10"
        />
        <StatsCard 
          title="Tasks Remaining" 
          value={(summary?.todayPlan || []).filter(p => !p.completed).length}
          icon={ListTodo}
          color="text-info"
          bg="bg-info/10"
        />
        <StatsCard 
          title="Pending Chores" 
          value={(summary?.pendingChores || []).length}
          icon={ClipboardList}
          color="text-chart-4"
          bg="bg-chart-4/10"
        />
      </div>

      {/* Coaching Card */}
      {coachingLoading ? (
        <Card className="border-border bg-card/50 backdrop-blur-sm shadow-sm animate-pulse">
           <CardContent className="p-6 flex gap-4 items-center">
              <div className="h-10 w-10 rounded-full bg-muted"></div>
              <div className="space-y-2 flex-1">
                <div className="h-4 w-32 bg-muted rounded-md"></div>
                <div className="h-3 w-3/4 bg-muted rounded-md"></div>
              </div>
           </CardContent>
        </Card>
      ) : coaching ? (
        <Card className="border-primary/20 bg-primary/5 backdrop-blur-sm shadow-sm relative overflow-hidden group">
          <div className="absolute top-0 right-0 p-8 opacity-10 pointer-events-none group-hover:opacity-20 transition-opacity">
            <Sparkles className="w-32 h-32 text-primary" />
          </div>
          <CardContent className="p-6 flex items-start gap-4 relative z-10">
            <div className="h-10 w-10 rounded-full bg-primary/20 flex items-center justify-center shrink-0 border border-primary/30">
              <Sparkles className="h-5 w-5 text-primary" />
            </div>
            <div>
              <h3 className="font-display font-medium text-primary mb-1 text-lg">Your Daily Insight</h3>
              <p className="text-sm text-foreground/90 leading-relaxed max-w-4xl">{coaching}</p>
            </div>
          </CardContent>
        </Card>
      ) : null}

      <div className="grid grid-cols-1 lg:grid-cols-3 gap-6">
        {/* Left Column - Plan & Actions */}
        <div className="lg:col-span-2 space-y-6">
          <Card className="border-border bg-card/50 backdrop-blur-sm shadow-sm">
            <CardHeader className="flex flex-row items-center justify-between pb-2 border-b border-border/50 mb-4">
              <CardTitle className="text-lg font-display flex items-center gap-2">
                <ListTodo className="h-5 w-5 text-primary" />
                Today's Plan
              </CardTitle>
              <Link href="/plan" className="text-sm text-muted-foreground hover:text-primary transition-colors flex items-center gap-1 group">
                View all <ArrowRight className="h-3 w-3 group-hover:translate-x-1 transition-transform" />
              </Link>
            </CardHeader>
            <CardContent>
              {summary?.todayPlan && summary.todayPlan.length > 0 ? (
                <div className="space-y-3">
                  {summary.todayPlan.map(plan => (
                    <div key={plan.id} className="flex items-center justify-between p-3 rounded-xl border border-border bg-muted/40 hover:bg-accent transition-colors">
                      <div className="flex items-center gap-3">
                        <div className={`h-5 w-5 rounded-full border flex items-center justify-center ${plan.completed ? 'bg-primary border-primary text-primary-foreground' : 'border-muted-foreground/50'}`}>
                          {plan.completed && <CheckCircle2 className="h-3 w-3" />}
                        </div>
                        <span className={plan.completed ? 'line-through text-muted-foreground' : 'font-medium'}>{plan.title}</span>
                      </div>
                      {plan.timeBlock && (
                        <span className="text-xs text-muted-foreground bg-muted px-2 py-1 rounded-md border border-border">{plan.timeBlock}</span>
                      )}
                    </div>
                  ))}
                </div>
              ) : (
                <div className="text-center py-10 bg-muted/40 rounded-xl border border-dashed border-border">
                  <p className="text-muted-foreground text-sm">No plan created for today.</p>
                  <Button variant="link" className="text-primary mt-2" asChild>
                    <Link href="/plan">Create your plan</Link>
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          <Card className="border-border bg-card/50 backdrop-blur-sm shadow-sm">
            <CardHeader className="flex flex-row items-center justify-between pb-2 border-b border-border/50 mb-4">
              <CardTitle className="text-lg font-display flex items-center gap-2">
                <Zap className="h-5 w-5 text-warning" />
                Open Actions
              </CardTitle>
              <Link href="/actions" className="text-sm text-muted-foreground hover:text-primary transition-colors flex items-center gap-1 group">
                View all <ArrowRight className="h-3 w-3 group-hover:translate-x-1 transition-transform" />
              </Link>
            </CardHeader>
            <CardContent>
              {summary?.openActionItems && summary.openActionItems.length > 0 ? (
                <div className="space-y-1">
                  {summary.openActionItems.slice(0, 5).map(action => (
                    <div key={action.id} className="flex items-start gap-3 p-3 rounded-lg hover:bg-accent transition-colors">
                      <div className="mt-0.5 h-4 w-4 rounded border border-muted-foreground/50 shrink-0" />
                      <div className="flex-1">
                        <p className="text-sm font-medium">{action.title}</p>
                        {action.dueDate && (
                          <p className="text-xs text-muted-foreground mt-1">
                            Due {formatDate(new Date(action.dueDate), { month: 'short', day: 'numeric' })}
                          </p>
                        )}
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <div className="text-center py-8 text-sm text-muted-foreground">
                  You have no open action items! 
                </div>
              )}
            </CardContent>
          </Card>
        </div>

        {/* Right Column - Habits & Events */}
        <div className="space-y-6">
          <Card className="border-border bg-card/50 backdrop-blur-sm shadow-sm">
            <CardHeader className="flex flex-row items-center justify-between pb-2 border-b border-border/50 mb-4">
              <CardTitle className="text-lg font-display flex items-center gap-2">
                <Flame className="h-5 w-5 text-warning" />
                Habits
              </CardTitle>
            </CardHeader>
            <CardContent>
              {summary?.habits && summary.habits.length > 0 ? (
                <div className="space-y-4">
                  {summary.habits.map(habit => (
                    <div key={habit.id} className="flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <div className={`h-10 w-10 rounded-xl flex items-center justify-center border ${habit.completedToday ? 'bg-primary/20 border-primary/30 text-primary' : 'bg-muted border-border text-muted-foreground'}`}>
                          {habit.icon ? <span className="text-sm">{habit.icon.substring(0, 2)}</span> : <CheckCircle2 className="h-5 w-5" />}
                        </div>
                        <span className="text-sm font-medium">{habit.name}</span>
                      </div>
                      <div className="flex items-center gap-1.5 text-xs font-medium">
                        <Flame className={`h-4 w-4 ${habit.currentStreak > 0 ? 'text-warning fill-warning/20' : 'text-muted-foreground/30'}`} />
                        <span className={habit.currentStreak > 0 ? 'text-foreground' : 'text-muted-foreground'}>
                          {habit.currentStreak}
                        </span>
                      </div>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-center py-6 text-sm text-muted-foreground">No habits tracked.</p>
              )}
            </CardContent>
          </Card>

          <Card className="border-border bg-card/50 backdrop-blur-sm shadow-sm">
            <CardHeader className="flex flex-row items-center justify-between pb-2 border-b border-border/50 mb-4">
              <CardTitle className="text-lg font-display flex items-center gap-2">
                <CalendarIcon className="h-5 w-5 text-info" />
                Upcoming
              </CardTitle>
            </CardHeader>
            <CardContent>
              {summary?.upcomingEvents && summary.upcomingEvents.length > 0 ? (
                <div className="space-y-4">
                  {summary.upcomingEvents.slice(0, 4).map(event => (
                    <div key={event.id} className="border-s-2 ps-3 py-1 text-sm bg-gradient-to-r from-white/5 to-transparent rounded-e-md" style={{ borderColor: event.color || 'hsl(var(--primary))' }}>
                      <p className="font-medium" dir="auto">{event.title}</p>
                      <p className="text-xs text-muted-foreground mt-1">
                        {isToday(new Date(event.startDate)) ? 'Today' : formatDate(new Date(event.startDate), { month: 'short', day: 'numeric' })}
                        {!event.allDay && event.startTime ? <> · <bdi dir="ltr">{event.startTime}</bdi></> : ''}
                      </p>
                    </div>
                  ))}
                </div>
              ) : (
                <p className="text-center py-6 text-sm text-muted-foreground">No upcoming events.</p>
              )}
            </CardContent>
          </Card>
        </div>
      </div>
    </PageTransition>
  );
}

function StatsCard({ title, value, icon: Icon, color, bg }: { title: string, value: string | number, icon: any, color: string, bg: string }) {
  return (
    <Card className="border-border bg-card/40 backdrop-blur-sm overflow-hidden relative group transition-all hover:bg-card/60">
      <div className={`absolute -right-6 -top-6 opacity-[0.03] group-hover:opacity-10 transition-opacity duration-500 ${color}`}>
        <Icon className="w-32 h-32" />
      </div>
      <CardContent className="p-6 relative z-10">
        <div className="flex items-start justify-between">
          <div>
            <p className="text-sm font-medium text-muted-foreground mb-2">{title}</p>
            <p className="text-3xl font-display font-bold">{value}</p>
          </div>
          <div className={`h-12 w-12 rounded-2xl flex items-center justify-center border border-border shadow-inner ${bg} ${color}`}>
            <Icon className="h-6 w-6" />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

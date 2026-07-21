import { useState, useEffect } from 'react';
import { motion } from 'framer-motion';
import { useGetDashboardSummary, useGetGoogleStatus } from '@workspace/api-client-react';
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
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Link } from 'wouter';
import { format, isToday } from 'date-fns';
import { cn } from '@/lib/utils';
import { PageTransition } from '@/components/ui/page-transition';

export function DashboardPage() {
  const { data: summary, isLoading } = useGetDashboardSummary();
  useGetGoogleStatus(); // keep data warm so googleConnection.calendarConnected is populated
  const [coaching, setCoaching] = useState<string | null>(null);
  const [coachingLoading, setCoachingLoading] = useState(true);

  useEffect(() => {
    if (!summary) return;
    fetch('/api/ai/coaching', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      credentials: 'include',
      body: JSON.stringify({
        habits: summary.habits,
        goals: summary.goals,
        habitsCompletedToday: summary.habitsCompletedToday,
        habitsTotal: summary.habitsTotal,
      }),
    })
      .then(r => r.ok ? r.json() : null)
      .then(data => { if (data?.message) setCoaching(data.message); })
      .catch(() => {})
      .finally(() => setCoachingLoading(false));
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
      <header>
        <h1 className="text-3xl md:text-4xl font-display font-bold tracking-tight">Good morning.</h1>
        <p className="text-muted-foreground mt-2 text-lg">Here is your focus for today, {format(new Date(), 'EEEE, MMMM d')}.</p>
      </header>

      {/* Google status card */}
      <Card className={cn(
        'border-border bg-card/50 backdrop-blur-sm shadow-sm',
        googleConnected ? 'border-emerald-500/20' : 'border-amber-500/20'
      )}>
        <CardContent className="p-4 flex flex-col sm:flex-row sm:items-center justify-between gap-4">
          <div className="flex items-center gap-3">
            <div className={cn(
              'h-10 w-10 rounded-full flex items-center justify-center border',
              googleConnected ? 'bg-emerald-500/10 border-emerald-500/30 text-emerald-500' : 'bg-amber-500/10 border-amber-500/30 text-amber-500'
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
          color="text-emerald-500"
          bg="bg-emerald-500/10"
        />
        <StatsCard 
          title="Active Goals" 
          value={summary?.goalsActive || 0}
          icon={Target}
          color="text-amber-500"
          bg="bg-amber-500/10"
        />
        <StatsCard 
          title="Tasks Remaining" 
          value={(summary?.todayPlan || []).filter(p => !p.completed).length}
          icon={ListTodo}
          color="text-blue-500"
          bg="bg-blue-500/10"
        />
        <StatsCard 
          title="Pending Chores" 
          value={(summary?.pendingChores || []).length}
          icon={ClipboardList}
          color="text-purple-500"
          bg="bg-purple-500/10"
        />
      </div>

      {/* Coaching Card */}
      {coachingLoading ? (
        <Card className="border-border bg-card/50 backdrop-blur-sm shadow-sm animate-pulse">
           <CardContent className="p-6 flex gap-4 items-center">
              <div className="h-10 w-10 rounded-full bg-white/5"></div>
              <div className="space-y-2 flex-1">
                <div className="h-4 w-32 bg-white/5 rounded-md"></div>
                <div className="h-3 w-3/4 bg-white/5 rounded-md"></div>
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
                    <div key={plan.id} className="flex items-center justify-between p-3 rounded-xl border border-white/5 bg-white/5 hover:bg-white/10 transition-colors">
                      <div className="flex items-center gap-3">
                        <div className={`h-5 w-5 rounded-full border flex items-center justify-center ${plan.completed ? 'bg-primary border-primary text-primary-foreground' : 'border-muted-foreground/50'}`}>
                          {plan.completed && <CheckCircle2 className="h-3 w-3" />}
                        </div>
                        <span className={plan.completed ? 'line-through text-muted-foreground' : 'font-medium'}>{plan.title}</span>
                      </div>
                      {plan.timeBlock && (
                        <span className="text-xs text-muted-foreground bg-black/20 px-2 py-1 rounded-md border border-white/5">{plan.timeBlock}</span>
                      )}
                    </div>
                  ))}
                </div>
              ) : (
                <div className="text-center py-10 bg-black/20 rounded-xl border border-dashed border-white/10">
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
                <Zap className="h-5 w-5 text-amber-500" />
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
                    <div key={action.id} className="flex items-start gap-3 p-3 rounded-lg hover:bg-white/5 transition-colors">
                      <div className="mt-0.5 h-4 w-4 rounded border border-muted-foreground/50 shrink-0" />
                      <div className="flex-1">
                        <p className="text-sm font-medium">{action.title}</p>
                        {action.dueDate && (
                          <p className="text-xs text-muted-foreground mt-1">
                            Due {format(new Date(action.dueDate), 'MMM d')}
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
                <Flame className="h-5 w-5 text-orange-500" />
                Habits
              </CardTitle>
            </CardHeader>
            <CardContent>
              {summary?.habits && summary.habits.length > 0 ? (
                <div className="space-y-4">
                  {summary.habits.map(habit => (
                    <div key={habit.id} className="flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        <div className={`h-10 w-10 rounded-xl flex items-center justify-center border ${habit.completedToday ? 'bg-primary/20 border-primary/30 text-primary' : 'bg-black/20 border-white/5 text-muted-foreground'}`}>
                          {habit.icon ? <span className="text-sm">{habit.icon.substring(0, 2)}</span> : <CheckCircle2 className="h-5 w-5" />}
                        </div>
                        <span className="text-sm font-medium">{habit.name}</span>
                      </div>
                      <div className="flex items-center gap-1.5 text-xs font-medium">
                        <Flame className={`h-4 w-4 ${habit.currentStreak > 0 ? 'text-orange-500 fill-orange-500/20' : 'text-muted-foreground/30'}`} />
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
                <CalendarIcon className="h-5 w-5 text-blue-400" />
                Upcoming
              </CardTitle>
            </CardHeader>
            <CardContent>
              {summary?.upcomingEvents && summary.upcomingEvents.length > 0 ? (
                <div className="space-y-4">
                  {summary.upcomingEvents.slice(0, 4).map(event => (
                    <div key={event.id} className="border-l-2 pl-3 py-1 text-sm bg-gradient-to-r from-white/5 to-transparent rounded-r-md" style={{ borderColor: event.color || 'hsl(var(--primary))' }}>
                      <p className="font-medium">{event.title}</p>
                      <p className="text-xs text-muted-foreground mt-1">
                        {isToday(new Date(event.startDate)) ? 'Today' : format(new Date(event.startDate), 'MMM d')}
                        {!event.allDay && event.startTime ? `, ${event.startTime}` : ''}
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
          <div className={`h-12 w-12 rounded-2xl flex items-center justify-center border border-white/5 shadow-inner ${bg} ${color}`}>
            <Icon className="h-6 w-6" />
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

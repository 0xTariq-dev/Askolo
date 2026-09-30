import { useState, useMemo } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { useQueryClient } from '@tanstack/react-query';
import { format, subDays, addDays, isSameDay } from 'date-fns';
import { useLocale } from '@/contexts/locale-context';
import { z } from 'zod';
import { useForm as useHookForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { 
  Flame, 
  Plus, 
  MoreVertical, 
  Trash2, 
  Edit2, 
  Activity,
  CheckCircle2,
  Calendar
} from 'lucide-react';

import {
  useListHabits,
  useCreateHabit,
  useUpdateHabit,
  useDeleteHabit,
  useCompleteHabit,
  useUncompleteHabit,
  useGetHabitCompletions,
  getListHabitsQueryKey,
  getGetDashboardSummaryQueryKey,
  getGetHabitCompletionsQueryKey,
  Habit
} from '@workspace/api-client-react';

import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { PageTransition } from '@/components/ui/page-transition';

const PRESET_COLORS = [
  { name: 'Rose', value: 'hsl(346 87% 60%)' },
  { name: 'Amber', value: 'hsl(38 92% 50%)' },
  { name: 'Emerald', value: 'hsl(142 71% 45%)' },
  { name: 'Blue', value: 'hsl(217 91% 60%)' },
  { name: 'Indigo', value: 'hsl(239 84% 67%)' },
  { name: 'Purple', value: 'hsl(271 91% 65%)' },
];

const habitSchema = z.object({
  name: z.string().min(1, "Name is required"),
  description: z.string().optional(),
  frequency: z.enum(['daily', 'weekly']),
  color: z.string().optional(),
});

type HabitFormValues = z.infer<typeof habitSchema>;

export function HabitsPage() {
  const qc = useQueryClient();
  const { data: habits, isLoading } = useListHabits();
  
  const [selectedHabitId, setSelectedHabitId] = useState<number | null>(null);
  
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingHabit, setEditingHabit] = useState<Habit | null>(null);

  const createHabit = useCreateHabit();
  const updateHabit = useUpdateHabit();
  const deleteHabit = useDeleteHabit();
  const completeHabit = useCompleteHabit();
  const uncompleteHabit = useUncompleteHabit();

  const form = useHookForm<HabitFormValues>({
    resolver: zodResolver(habitSchema),
    defaultValues: {
      name: '',
      description: '',
      frequency: 'daily',
      color: PRESET_COLORS[1].value,
    }
  });

  const openAdd = () => {
    setEditingHabit(null);
    form.reset({
      name: '',
      description: '',
      frequency: 'daily',
      color: PRESET_COLORS[1].value,
    });
    setDialogOpen(true);
  };

  const openEdit = (habit: Habit) => {
    setEditingHabit(habit);
    form.reset({
      name: habit.name,
      description: habit.description || '',
      frequency: habit.frequency,
      color: habit.color || PRESET_COLORS[1].value,
    });
    setDialogOpen(true);
  };

  const onSubmit = (data: HabitFormValues) => {
    if (editingHabit) {
      updateHabit.mutate({
        id: editingHabit.id,
        data,
      }, {
        onSuccess: () => {
          qc.invalidateQueries({ queryKey: getListHabitsQueryKey() });
          qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
          setDialogOpen(false);
        }
      });
    } else {
      createHabit.mutate({
        data,
      }, {
        onSuccess: () => {
          qc.invalidateQueries({ queryKey: getListHabitsQueryKey() });
          qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
          setDialogOpen(false);
        }
      });
    }
  };

  const handleDelete = (id: number) => {
    if (confirm("Delete this habit?")) {
      deleteHabit.mutate({ id }, {
        onSuccess: () => {
          qc.invalidateQueries({ queryKey: getListHabitsQueryKey() });
          qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
          if (selectedHabitId === id) setSelectedHabitId(null);
        }
      });
    }
  };

  const toggleToday = (habit: Habit) => {
    const today = format(new Date(), 'yyyy-MM-dd');
    const isCompleted = habit.completedToday;
    
    // Optimistic update
    qc.setQueryData(getListHabitsQueryKey(), (old: Habit[] | undefined) => {
      if (!old) return old;
      return old.map(h => {
        if (h.id === habit.id) {
          return {
            ...h,
            completedToday: !isCompleted,
            currentStreak: isCompleted ? Math.max(0, h.currentStreak - 1) : h.currentStreak + 1,
            longestStreak: !isCompleted && h.currentStreak + 1 > h.longestStreak ? h.currentStreak + 1 : h.longestStreak
          };
        }
        return h;
      });
    });

    const options = {
      onSuccess: () => {
        qc.invalidateQueries({ queryKey: getListHabitsQueryKey() });
        qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
      },
      onError: () => {
        qc.invalidateQueries({ queryKey: getListHabitsQueryKey() });
      }
    };

    if (isCompleted) {
      uncompleteHabit.mutate({ id: habit.id, date: today }, options);
    } else {
      completeHabit.mutate({ id: habit.id, data: { date: today } }, options);
    }
  };

  const handleHabitHover = (id: number) => {
    setSelectedHabitId(id);
  };

  const activeHabitForHeatmap = habits?.find(h => h.id === selectedHabitId) || habits?.[0] || null;

  return (
    <PageTransition className="space-y-8 max-w-5xl mx-auto pb-10">
      <header className="flex flex-col sm:flex-row sm:items-end justify-between gap-4">
        <div>
          <h1 className="text-3xl md:text-4xl font-display font-bold tracking-tight">Habits Tracker</h1>
          <p className="text-muted-foreground mt-2 text-lg">Build consistency and track your daily routines.</p>
        </div>
        <Button onClick={openAdd} className="shrink-0" data-testid="button-add-habit">
          <Plus className="h-4 w-4 mr-2" /> Add Habit
        </Button>
      </header>

      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {[...Array(4)].map((_, i) => (
            <div key={i} className="h-28 bg-card rounded-xl animate-pulse" />
          ))}
        </div>
      ) : habits && habits.length > 0 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {habits.map((habit) => (
            <HabitCard 
              key={habit.id} 
              habit={habit} 
              onToggle={() => toggleToday(habit)}
              onEdit={() => openEdit(habit)}
              onDelete={() => handleDelete(habit.id)}
              onHover={() => handleHabitHover(habit.id)}
              isSelected={selectedHabitId === habit.id || (selectedHabitId === null && habits[0].id === habit.id)}
            />
          ))}
        </div>
      ) : (
        <div className="text-center py-20 bg-card/50 rounded-2xl border border-dashed border-border/60 backdrop-blur-sm">
          <div className="h-16 w-16 bg-primary/10 rounded-full flex items-center justify-center mx-auto mb-4">
            <Activity className="h-8 w-8 text-primary" />
          </div>
          <h2 className="text-xl font-display font-semibold mb-2">No habits yet</h2>
          <p className="text-muted-foreground mb-6 max-w-md mx-auto">
            Start small. Add a daily routine you want to build and track your consistency over time.
          </p>
          <Button onClick={openAdd}>
            <Plus className="h-4 w-4 mr-2" /> Create Your First Habit
          </Button>
        </div>
      )}

      {activeHabitForHeatmap && (
        <div className="mt-8">
          <HeatmapSection habit={activeHabitForHeatmap} />
        </div>
      )}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-[425px]">
          <DialogHeader>
            <DialogTitle>{editingHabit ? 'Edit Habit' : 'New Habit'}</DialogTitle>
          </DialogHeader>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="name">Habit Name</Label>
              <Input 
                id="name" 
                placeholder="e.g. Read for 20 minutes" 
                {...form.register('name')} 
                data-testid="input-habit-name"
              />
              {form.formState.errors.name && (
                <p className="text-xs text-destructive">{form.formState.errors.name.message}</p>
              )}
            </div>
            
            <div className="space-y-2">
              <Label htmlFor="description">Description (optional)</Label>
              <Textarea 
                id="description" 
                placeholder="Why do you want to build this habit?" 
                {...form.register('description')} 
                className="resize-none"
              />
            </div>
            
            <div className="space-y-2">
              <Label>Frequency</Label>
              <div className="flex gap-4">
                <label className="flex items-center gap-2 text-sm cursor-pointer">
                  <input type="radio" value="daily" {...form.register('frequency')} className="accent-primary" />
                  Daily
                </label>
                <label className="flex items-center gap-2 text-sm cursor-pointer">
                  <input type="radio" value="weekly" {...form.register('frequency')} className="accent-primary" />
                  Weekly
                </label>
              </div>
            </div>

            <div className="space-y-2">
              <Label>Color Indicator</Label>
              <div className="flex gap-3">
                {PRESET_COLORS.map(color => (
                  <button
                    key={color.value}
                    type="button"
                    onClick={() => form.setValue('color', color.value)}
                    className={`h-8 w-8 rounded-full transition-transform ${form.watch('color') === color.value ? 'scale-110 ring-2 ring-offset-2 ring-offset-background ring-border' : 'hover:scale-105'}`}
                    style={{ backgroundColor: color.value }}
                    title={color.name}
                  />
                ))}
              </div>
            </div>

            <DialogFooter className="pt-4">
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={form.formState.isSubmitting}>
                {form.formState.isSubmitting ? 'Saving...' : editingHabit ? 'Save Changes' : 'Create Habit'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </PageTransition>
  );
}

function HabitCard({ habit, onToggle, onEdit, onDelete, onHover, isSelected }: { 
  habit: Habit, 
  onToggle: () => void, 
  onEdit: () => void, 
  onDelete: () => void,
  onHover: () => void,
  isSelected: boolean
}) {
  const isCompleted = habit.completedToday;
  const color = habit.color || 'hsl(var(--primary))';

  return (
    <Card 
      className={`group relative overflow-hidden transition-all duration-300 ${isSelected ? 'ring-1 ring-white/10 shadow-lg bg-card/80' : 'bg-card/40 hover:bg-card/60'}`}
      onMouseEnter={onHover}
      onClick={onHover}
    >
      <div 
        className="absolute left-0 top-0 bottom-0 w-1.5 transition-colors" 
        style={{ backgroundColor: color }}
      />
      <CardContent className="p-5 pl-6 flex items-center justify-between gap-4">
        <div className="flex items-center gap-4 flex-1 overflow-hidden">
          <motion.button
            whileTap={{ scale: 0.9 }}
            onClick={(e) => { e.stopPropagation(); onToggle(); }}
            className={`shrink-0 h-12 w-12 rounded-xl flex items-center justify-center border transition-colors ${
              isCompleted 
                ? 'bg-primary/20 border-primary/30 text-primary' 
                : 'bg-muted border-border text-muted-foreground hover:bg-accent hover:border-primary/30'
            }`}
            data-testid={`toggle-habit-${habit.id}`}
          >
            <AnimatePresence mode="wait">
              {isCompleted ? (
                <motion.div
                  key="check"
                  initial={{ scale: 0, opacity: 0 }}
                  animate={{ scale: 1, opacity: 1 }}
                  exit={{ scale: 0, opacity: 0 }}
                  transition={{ type: 'spring', stiffness: 500, damping: 30 }}
                >
                  <CheckCircle2 className="h-6 w-6" style={{ color: isCompleted ? color : undefined }} />
                </motion.div>
              ) : (
                <motion.div
                  key="empty"
                  initial={{ scale: 0, opacity: 0 }}
                  animate={{ scale: 1, opacity: 1 }}
                  exit={{ scale: 0, opacity: 0 }}
                >
                  <div className="h-4 w-4 rounded-full border-2 border-current opacity-30" />
                </motion.div>
              )}
            </AnimatePresence>
          </motion.button>
          
          <div className="flex-1 min-w-0">
            <h3 className={`font-medium truncate transition-colors ${isCompleted ? 'text-foreground' : 'text-foreground/80'}`}>
              {habit.name}
            </h3>
            {habit.description && (
              <p className="text-xs text-muted-foreground truncate mt-0.5">{habit.description}</p>
            )}
            <div className="flex items-center gap-3 mt-2 text-xs font-medium">
              <div className="flex items-center gap-1">
                <Flame className={`h-3.5 w-3.5 ${habit.currentStreak > 0 ? 'text-warning fill-warning/20' : 'text-muted-foreground/30'}`} />
                <span className={habit.currentStreak > 0 ? 'text-foreground/90' : 'text-muted-foreground'}>
                  {habit.currentStreak} {habit.currentStreak === 1 ? 'day' : 'days'}
                </span>
              </div>
              <span className="text-muted-foreground/40">•</span>
              <span className="text-muted-foreground">Longest: {habit.longestStreak}</span>
            </div>
          </div>
        </div>

        <DropdownMenu>
          <DropdownMenuTrigger asChild onClick={e => e.stopPropagation()}>
            <Button variant="ghost" size="icon" className="h-8 w-8 opacity-0 group-hover:opacity-100 transition-opacity">
              <MoreVertical className="h-4 w-4 text-muted-foreground" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-40">
            <DropdownMenuItem onClick={onEdit}>
              <Edit2 className="h-4 w-4 mr-2" /> Edit
            </DropdownMenuItem>
            <DropdownMenuItem onClick={onDelete} className="text-destructive focus:text-destructive focus:bg-destructive/10">
              <Trash2 className="h-4 w-4 mr-2" /> Delete
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </CardContent>
    </Card>
  );
}

function HeatmapSection({ habit }: { habit: Habit }) {
  const { formatDate } = useLocale();
  const { data: completions, isLoading } = useGetHabitCompletions(habit.id, {
    query: {
      enabled: !!habit.id,
      queryKey: getGetHabitCompletionsQueryKey(habit.id),
    }
  });

  const heatmapColor = habit.color || 'hsl(var(--primary))';

  // Generate 13 weeks of data (91 days) ending today
  const today = new Date();
  const startDate = subDays(today, 90);
  
  const days = useMemo(() => {
    const arr = [];
    const completionSet = new Set(completions?.map(c => c.date.substring(0, 10)) || []);
    
    for (let i = 0; i < 91; i++) {
      const d = addDays(startDate, i);
      const dateStr = format(d, 'yyyy-MM-dd');
      arr.push({
        date: d,
        dateStr,
        completed: completionSet.has(dateStr),
        isToday: isSameDay(d, today)
      });
    }
    return arr;
  }, [completions, startDate, today]);

  // Group into columns (weeks)
  const columns = useMemo(() => {
    const cols = [];
    let currentWeek = [];
    
    for (let i = 0; i < days.length; i++) {
      currentWeek.push(days[i]);
      if (currentWeek.length === 7 || i === days.length - 1) {
        cols.push(currentWeek);
        currentWeek = [];
      }
    }
    return cols;
  }, [days]);

  return (
    <motion.div
      initial={{ opacity: 0, y: 10 }}
      animate={{ opacity: 1, y: 0 }}
      key={habit.id}
    >
      <Card className="border-border bg-card/30 backdrop-blur-sm overflow-hidden">
        <CardContent className="p-6">
          <div className="flex items-center justify-between mb-6">
            <h3 className="font-display font-medium flex items-center gap-2">
              <Calendar className="h-4 w-4 text-muted-foreground" /> 
              Activity: <span style={{ color: heatmapColor }}>{habit.name}</span>
            </h3>
            <span className="text-xs text-muted-foreground">Last 90 days</span>
          </div>
          
          {isLoading ? (
            <div className="h-24 w-full bg-muted animate-pulse rounded-md" />
          ) : (
            <div className="flex gap-1.5 overflow-x-auto pb-2 scrollbar-none">
              {columns.map((col, colIdx) => (
                <div key={colIdx} className="flex flex-col gap-1.5">
                  {col.map((day) => (
                    <div
                      key={day.dateStr}
                      title={`${formatDate(day.date, { month: 'short', day: 'numeric', year: 'numeric' })}${day.completed ? ' (Completed)' : ''}`}
                      className={`w-3.5 h-3.5 rounded-[3px] transition-colors ${
                        day.completed
                          ? 'opacity-100 shadow-sm'
                          : 'bg-muted opacity-40 hover:opacity-70'
                      } ${day.isToday && !day.completed ? 'ring-1 ring-white/20 ring-offset-1 ring-offset-background' : ''}`}
                      style={day.completed ? { backgroundColor: heatmapColor } : {}}
                    />
                  ))}
                </div>
              ))}
            </div>
          )}
        </CardContent>
      </Card>
    </motion.div>
  );
}

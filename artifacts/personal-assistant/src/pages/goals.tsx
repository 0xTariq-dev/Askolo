import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { format } from 'date-fns';
import { 
  Target, 
  Plus, 
  MoreVertical, 
  Trash2, 
  Edit2, 
  CheckCircle2,
  Calendar,
  Check
} from 'lucide-react';

import {
  useListGoals,
  useCreateGoal,
  useUpdateGoal,
  useDeleteGoal,
  getListGoalsQueryKey,
  getGetDashboardSummaryQueryKey,
  Goal
} from '@workspace/api-client-react';

import { Button } from '@workspace/askolo-design-system/components/ui/button';
import { Card, CardContent } from '@workspace/askolo-design-system/components/ui/card';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@workspace/askolo-design-system/components/ui/dialog';
import { Input } from '@workspace/askolo-design-system/components/ui/input';
import { Label } from '@workspace/askolo-design-system/components/ui/label';
import { Textarea } from '@workspace/askolo-design-system/components/ui/textarea';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@workspace/askolo-design-system/components/ui/dropdown-menu';
import { Slider } from '@workspace/askolo-design-system/components/ui/slider';
import { PageTransition } from '@/components/ui/page-transition';

const CATEGORY_COLORS: Record<string, string> = {
  'Health': 'bg-success/10 text-success border-success/30',
  'Career': 'bg-info/10 text-info border-info/30',
  'Finance': 'bg-warning/10 text-warning border-warning/30',
  'Learning': 'bg-chart-4/10 text-chart-4 border-chart-4/30',
  'Personal': 'bg-chart-5/10 text-chart-5 border-chart-5/30',
  'Other': 'bg-muted text-muted-foreground border-border',
};

const goalSchema = z.object({
  title: z.string().min(1, "Title is required"),
  description: z.string().optional(),
  category: z.string().optional(),
  targetDate: z.string().optional(),
  progress: z.coerce.number().min(0).max(100),
  status: z.enum(['active', 'paused', 'completed']),
});

type GoalFormValues = z.infer<typeof goalSchema>;

export function GoalsPage() {
  const qc = useQueryClient();
  const { data: goals, isLoading } = useListGoals();
  
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingGoal, setEditingGoal] = useState<Goal | null>(null);

  const createGoal = useCreateGoal();
  const updateGoal = useUpdateGoal();
  const deleteGoal = useDeleteGoal();

  const form = useForm<GoalFormValues>({
    resolver: zodResolver(goalSchema),
    defaultValues: {
      title: '',
      description: '',
      category: 'Other',
      targetDate: '',
      progress: 0,
      status: 'active',
    }
  });

  const openAdd = () => {
    setEditingGoal(null);
    form.reset({
      title: '',
      description: '',
      category: 'Other',
      targetDate: '',
      progress: 0,
      status: 'active',
    });
    setDialogOpen(true);
  };

  const openEdit = (goal: Goal) => {
    setEditingGoal(goal);
    form.reset({
      title: goal.title,
      description: goal.description || '',
      category: goal.category || 'Other',
      targetDate: goal.targetDate ? goal.targetDate.substring(0, 10) : '',
      progress: goal.progress,
      status: goal.status,
    });
    setDialogOpen(true);
  };

  const onSubmit = (data: GoalFormValues) => {
    // If completed and progress < 100, auto-bump to 100
    if (data.status === 'completed' && data.progress < 100) {
      data.progress = 100;
    }
    // If progress is 100, auto-complete
    if (data.progress === 100 && data.status === 'active') {
      data.status = 'completed';
    }

    // Convert empty targetDate back to undefined to avoid API errors
    const payload = { ...data };
    if (!payload.targetDate) delete payload.targetDate;

    if (editingGoal) {
      updateGoal.mutate({
        id: editingGoal.id,
        data: payload,
      }, {
        onSuccess: () => {
          qc.invalidateQueries({ queryKey: getListGoalsQueryKey() });
          qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
          setDialogOpen(false);
        }
      });
    } else {
      createGoal.mutate({
        data: payload,
      }, {
        onSuccess: () => {
          qc.invalidateQueries({ queryKey: getListGoalsQueryKey() });
          qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
          setDialogOpen(false);
        }
      });
    }
  };

  const handleDelete = (id: number) => {
    if (confirm("Delete this goal?")) {
      deleteGoal.mutate({ id }, {
        onSuccess: () => {
          qc.invalidateQueries({ queryKey: getListGoalsQueryKey() });
          qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
        }
      });
    }
  };

  const incrementProgress = (goal: Goal, amount: number) => {
    const newProgress = Math.min(100, goal.progress + amount);
    const newStatus = newProgress === 100 ? 'completed' : goal.status;
    
    // Optimistic update
    qc.setQueryData(getListGoalsQueryKey(), (old: Goal[] | undefined) => {
      if (!old) return old;
      return old.map(g => hIdMatch(g, goal.id, { progress: newProgress, status: newStatus }));
    });

    updateGoal.mutate({
      id: goal.id,
      data: { progress: newProgress, status: newStatus }
    }, {
      onSuccess: () => {
        qc.invalidateQueries({ queryKey: getListGoalsQueryKey() });
        qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
      }
    });
  };

  const markComplete = (goal: Goal) => {
    // Optimistic update
    qc.setQueryData(getListGoalsQueryKey(), (old: Goal[] | undefined) => {
      if (!old) return old;
      return old.map(g => hIdMatch(g, goal.id, { progress: 100, status: 'completed' }));
    });

    updateGoal.mutate({
      id: goal.id,
      data: { status: 'completed', progress: 100 }
    }, {
      onSuccess: () => {
        qc.invalidateQueries({ queryKey: getListGoalsQueryKey() });
        qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
      }
    });
  };

  const hIdMatch = (g: Goal, id: number, updates: any) => g.id === id ? { ...g, ...updates } : g;

  const sortedGoals = goals ? [...goals].sort((a, b) => {
    const statusOrder = { active: 0, paused: 1, completed: 2 };
    if (statusOrder[a.status] !== statusOrder[b.status]) {
      return statusOrder[a.status] - statusOrder[b.status];
    }
    return b.progress - a.progress;
  }) : [];

  const activeCount = goals?.filter(g => g.status === 'active').length || 0;
  const completedCount = goals?.filter(g => g.status === 'completed').length || 0;
  const pausedCount = goals?.filter(g => g.status === 'paused').length || 0;

  return (
    <PageTransition className="space-y-8 max-w-5xl mx-auto pb-10">
      <header className="flex flex-col sm:flex-row sm:items-end justify-between gap-4">
        <div>
          <h1 className="text-3xl md:text-4xl font-display font-bold tracking-tight">Your Growth Journey</h1>
          <div className="flex items-center gap-3 mt-3 text-sm text-muted-foreground font-medium">
            <span className="flex items-center gap-1.5"><span className="h-2 w-2 rounded-full bg-primary" /> {activeCount} Active</span>
            <span className="text-muted-foreground/30">•</span>
            <span className="flex items-center gap-1.5"><span className="h-2 w-2 rounded-full bg-success" /> {completedCount} Completed</span>
            <span className="text-muted-foreground/30">•</span>
            <span className="flex items-center gap-1.5"><span className="h-2 w-2 rounded-full bg-muted-foreground/50" /> {pausedCount} Paused</span>
          </div>
        </div>
        <Button onClick={openAdd} className="shrink-0" data-testid="button-add-goal">
          <Plus className="h-4 w-4 mr-2" /> New Goal
        </Button>
      </header>

      {isLoading ? (
        <div className="grid grid-cols-1 gap-4">
          {[...Array(3)].map((_, i) => (
            <div key={i} className="h-40 bg-card rounded-xl animate-pulse" />
          ))}
        </div>
      ) : sortedGoals.length > 0 ? (
        <div className="grid grid-cols-1 gap-6">
          {sortedGoals.map((goal) => (
            <GoalCard 
              key={goal.id} 
              goal={goal} 
              onEdit={() => openEdit(goal)}
              onDelete={() => handleDelete(goal.id)}
              onIncrement={() => incrementProgress(goal, 10)}
              onComplete={() => markComplete(goal)}
            />
          ))}
        </div>
      ) : (
        <div className="text-center py-20 bg-card/50 rounded-2xl border border-dashed border-border/60 backdrop-blur-sm">
          <div className="h-16 w-16 bg-primary/10 rounded-full flex items-center justify-center mx-auto mb-4">
            <Target className="h-8 w-8 text-primary" />
          </div>
          <h2 className="text-xl font-display font-semibold mb-2">Set your sights</h2>
          <p className="text-muted-foreground mb-6 max-w-md mx-auto">
            Define your long-term goals and track your progress along the way. Break big dreams into measurable steps.
          </p>
          <Button onClick={openAdd}>
            <Plus className="h-4 w-4 mr-2" /> Create Your First Goal
          </Button>
        </div>
      )}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-[500px]">
          <DialogHeader>
            <DialogTitle>{editingGoal ? 'Edit Goal' : 'New Goal'}</DialogTitle>
          </DialogHeader>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="title">Goal Title</Label>
              <Input 
                id="title" 
                placeholder="e.g. Read 50 books this year" 
                {...form.register('title')} 
              />
              {form.formState.errors.title && (
                <p className="text-xs text-destructive">{form.formState.errors.title.message}</p>
              )}
            </div>
            
            <div className="space-y-2">
              <Label htmlFor="description">Description (optional)</Label>
              <Textarea 
                id="description" 
                placeholder="What is the outcome you want?" 
                {...form.register('description')} 
                className="resize-none"
              />
            </div>
            
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="category">Category</Label>
                <select 
                  className="flex h-10 w-full items-center justify-between rounded-md border border-input bg-transparent px-3 py-2 text-sm shadow-sm ring-offset-background placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring disabled:cursor-not-allowed disabled:opacity-50"
                  {...form.register('category')}
                >
                  <option className="bg-popover" value="Health">Health</option>
                  <option className="bg-popover" value="Career">Career</option>
                  <option className="bg-popover" value="Finance">Finance</option>
                  <option className="bg-popover" value="Learning">Learning</option>
                  <option className="bg-popover" value="Personal">Personal</option>
                  <option className="bg-popover" value="Other">Other</option>
                </select>
              </div>
              
              <div className="space-y-2">
                <Label htmlFor="targetDate">Target Date</Label>
                <Input 
                  id="targetDate" 
                  type="date"
                  {...form.register('targetDate')} 
                />
              </div>
            </div>

            <div className="space-y-4 pt-2">
              <div className="flex items-center justify-between">
                <Label>Current Progress: {form.watch('progress')}%</Label>
              </div>
              <Slider 
                value={[form.watch('progress')]} 
                onValueChange={([v]) => form.setValue('progress', v)} 
                max={100} 
                step={1} 
              />
            </div>

            <div className="space-y-2">
              <Label>Status</Label>
              <div className="flex gap-4">
                <label className="flex items-center gap-2 text-sm cursor-pointer">
                  <input type="radio" value="active" {...form.register('status')} className="accent-primary" />
                  Active
                </label>
                <label className="flex items-center gap-2 text-sm cursor-pointer">
                  <input type="radio" value="paused" {...form.register('status')} className="accent-primary" />
                  Paused
                </label>
                <label className="flex items-center gap-2 text-sm cursor-pointer">
                  <input type="radio" value="completed" {...form.register('status')} className="accent-primary" />
                  Completed
                </label>
              </div>
            </div>

            <DialogFooter className="pt-4">
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>Cancel</Button>
              <Button type="submit" disabled={form.formState.isSubmitting}>
                {form.formState.isSubmitting ? 'Saving...' : editingGoal ? 'Save Changes' : 'Create Goal'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </PageTransition>
  );
}

function GoalCard({ goal, onEdit, onDelete, onIncrement, onComplete }: { 
  goal: Goal, 
  onEdit: () => void, 
  onDelete: () => void,
  onIncrement: () => void,
  onComplete: () => void
}) {
  const isCompleted = goal.status === 'completed';
  const categoryStyle = goal.category ? CATEGORY_COLORS[goal.category] || CATEGORY_COLORS['Other'] : CATEGORY_COLORS['Other'];
  
  return (
    <Card className={`group relative overflow-hidden transition-all duration-300 ${isCompleted ? 'bg-card/20 opacity-80 border-dashed' : 'bg-card/50 hover:bg-card/70'}`}>
      <CardContent className="p-6">
        <div className="flex flex-col md:flex-row md:items-start justify-between gap-6">
          <div className="flex-1 space-y-4">
            <div className="flex items-start justify-between">
              <div>
                <div className="flex items-center gap-2 mb-2">
                  <span className={`text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-full border ${categoryStyle}`}>
                    {goal.category || 'Goal'}
                  </span>
                  
                  {goal.status === 'active' && (
                    <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-full bg-warning/10 text-warning border border-warning/20">
                      Active
                    </span>
                  )}
                  {goal.status === 'paused' && (
                    <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-full bg-muted text-muted-foreground border border-border">
                      Paused
                    </span>
                  )}
                  {isCompleted && (
                    <span className="text-[10px] font-bold uppercase tracking-wider px-2 py-0.5 rounded-full bg-success/10 text-success border border-success/20 flex items-center gap-1">
                      <Check className="h-3 w-3" /> Completed
                    </span>
                  )}
                </div>
                <h3 className={`text-xl font-display font-semibold transition-colors ${isCompleted ? 'text-muted-foreground line-through decoration-white/20' : 'text-foreground'}`}>
                  {goal.title}
                </h3>
                {goal.description && (
                  <p className="text-sm text-muted-foreground mt-1 max-w-2xl">{goal.description}</p>
                )}
              </div>
            </div>

            <div className="space-y-2 max-w-xl">
              <div className="flex justify-between text-xs font-medium">
                <span className={isCompleted ? 'text-success' : 'text-primary'}>{goal.progress}% Complete</span>
                {goal.targetDate && (
                  <span className="text-muted-foreground flex items-center gap-1">
                    <Calendar className="h-3 w-3" /> Target: {format(new Date(goal.targetDate), 'MMM d, yyyy')}
                  </span>
                )}
              </div>
              <div className="h-2 w-full bg-muted rounded-full overflow-hidden border border-border relative">
                <div 
                  className={`absolute top-0 bottom-0 left-0 rounded-full transition-all duration-500 ease-out ${isCompleted ? 'bg-success' : 'bg-primary'}`}
                  style={{ width: `${goal.progress}%` }}
                />
              </div>
            </div>
          </div>

          <div className="flex md:flex-col items-center justify-end gap-2 shrink-0">
            {!isCompleted && (
              <>
                <Button variant="outline" size="sm" onClick={onIncrement} className="w-full md:w-auto h-8 text-xs border-primary/20 text-primary hover:bg-primary/10">
                  +10% Progress
                </Button>
                <Button variant="outline" size="sm" onClick={onComplete} className="w-full md:w-auto h-8 text-xs">
                  <CheckCircle2 className="h-3.5 w-3.5 mr-1.5" /> Mark Done
                </Button>
              </>
            )}
            
            <DropdownMenu>
              <DropdownMenuTrigger asChild>
                <Button variant="ghost" size="icon" className="h-8 w-8 ml-auto md:mt-2 opacity-50 hover:opacity-100">
                  <MoreVertical className="h-4 w-4" />
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align="end">
                <DropdownMenuItem onClick={onEdit}>
                  <Edit2 className="h-4 w-4 mr-2" /> Edit Goal
                </DropdownMenuItem>
                <DropdownMenuItem onClick={onDelete} className="text-destructive focus:text-destructive focus:bg-destructive/10">
                  <Trash2 className="h-4 w-4 mr-2" /> Delete
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

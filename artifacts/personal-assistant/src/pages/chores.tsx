import { useState } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { z } from 'zod';
import { useForm as useHookForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { format, isPast, startOfDay } from 'date-fns';
import { useLocale } from '@/contexts/locale-context';
import {
  ClipboardList,
  Plus,
  Trash2,
  Edit2,
  CheckCircle2,
  MoreVertical,
  Calendar,
  User,
  Repeat,
  Clock,
} from 'lucide-react';
import {
  useListChores,
  useCreateChore,
  useUpdateChore,
  useDeleteChore,
  useCompleteChore,
  getListChoresQueryKey,
  getGetDashboardSummaryQueryKey,
  Chore,
} from '@workspace/api-client-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent } from '@/components/ui/card';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Label } from '@/components/ui/label';
import { Textarea } from '@/components/ui/textarea';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu';
import { PageTransition } from '@/components/ui/page-transition';
import { cn } from '@/lib/utils';

const FREQUENCIES = ['once', 'daily', 'weekly', 'monthly'] as const;

const choreSchema = z.object({
  title: z.string().min(1, 'Title is required'),
  description: z.string().optional(),
  assignedTo: z.string().optional(),
  frequency: z.enum(FREQUENCIES),
  dueDate: z.string().optional(),
});

type ChoreFormValues = z.infer<typeof choreSchema>;

export function ChoresPage() {
  const qc = useQueryClient();
  const [filter, setFilter] = useState('all');
  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingChore, setEditingChore] = useState<Chore | null>(null);

  const { data: chores, isLoading } = useListChores();
  const createChore = useCreateChore();
  const updateChore = useUpdateChore();
  const deleteChore = useDeleteChore();
  const completeChore = useCompleteChore();

  const form = useHookForm<ChoreFormValues>({
    resolver: zodResolver(choreSchema),
    defaultValues: {
      title: '',
      description: '',
      assignedTo: '',
      frequency: 'weekly',
      dueDate: format(new Date(), 'yyyy-MM-dd'),
    },
  });

  const openAdd = () => {
    setEditingChore(null);
    form.reset({
      title: '',
      description: '',
      assignedTo: '',
      frequency: 'weekly',
      dueDate: format(new Date(), 'yyyy-MM-dd'),
    });
    setDialogOpen(true);
  };

  const openEdit = (chore: Chore) => {
    setEditingChore(chore);
    form.reset({
      title: chore.title,
      description: chore.description || '',
      assignedTo: chore.assignedTo || '',
      frequency: chore.frequency as any,
      dueDate: chore.dueDate || format(new Date(), 'yyyy-MM-dd'),
    });
    setDialogOpen(true);
  };

  const onSubmit = (data: ChoreFormValues) => {
    if (editingChore) {
      updateChore.mutate(
        { id: editingChore.id, data },
        {
          onSuccess: () => {
            qc.invalidateQueries({ queryKey: getListChoresQueryKey() });
            qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
            setDialogOpen(false);
          },
        },
      );
    } else {
      createChore.mutate(
        { data },
        {
          onSuccess: () => {
            qc.invalidateQueries({ queryKey: getListChoresQueryKey() });
            qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
            setDialogOpen(false);
          },
        },
      );
    }
  };

  const handleDelete = (id: number) => {
    if (confirm('Delete this chore?')) {
      deleteChore.mutate(
        { id },
        {
          onSuccess: () => {
            qc.invalidateQueries({ queryKey: getListChoresQueryKey() });
            qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
          },
        },
      );
    }
  };

  const toggleComplete = (chore: Chore) => {
    if (chore.completed) return;
    completeChore.mutate(
      { id: chore.id, data: { completedAt: new Date().toISOString() } },
      {
        onSuccess: () => {
          qc.invalidateQueries({ queryKey: getListChoresQueryKey() });
          qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
        },
      },
    );
  };

  const filteredChores = chores?.filter((chore) => {
    if (filter === 'completed') return chore.completed;
    if (filter === 'overdue') return !chore.completed && chore.dueDate && isPast(startOfDay(new Date(chore.dueDate)));
    if (filter === 'due') return !chore.completed;
    return true;
  });

  const counts = {
    all: chores?.length || 0,
    due: chores?.filter((c) => !c.completed).length || 0,
    overdue:
      chores?.filter((c) => !c.completed && c.dueDate && isPast(startOfDay(new Date(c.dueDate)))).length || 0,
    completed: chores?.filter((c) => c.completed).length || 0,
  };

  return (
    <PageTransition surface={false} className="space-y-8 max-w-5xl mx-auto pb-10">
      <header className="flex flex-col sm:flex-row sm:items-end justify-between gap-4">
        <div>
          <h1 className="text-3xl md:text-4xl font-display font-bold tracking-tight">Chores</h1>
          <p className="text-muted-foreground mt-2 text-lg">Recurring household tasks and assignments.</p>
        </div>
        <Button onClick={openAdd} className="shrink-0" data-testid="button-add-chore">
          <Plus className="h-4 w-4 mr-2" /> Add Chore
        </Button>
      </header>

      <Tabs value={filter} onValueChange={setFilter} className="w-full">
        <TabsList className="bg-card border border-border">
          <TabsTrigger value="all">All ({counts.all})</TabsTrigger>
          <TabsTrigger value="due">Due ({counts.due})</TabsTrigger>
          <TabsTrigger value="overdue">Overdue ({counts.overdue})</TabsTrigger>
          <TabsTrigger value="completed">Done ({counts.completed})</TabsTrigger>
        </TabsList>
      </Tabs>

      {isLoading ? (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {[...Array(4)].map((_, i) => (
            <div key={i} className="h-28 bg-card rounded-xl animate-pulse" />
          ))}
        </div>
      ) : filteredChores && filteredChores.length > 0 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          {filteredChores.map((chore) => (
            <ChoreCard
              key={chore.id}
              chore={chore}
              onEdit={() => openEdit(chore)}
              onDelete={() => handleDelete(chore.id)}
              onToggle={() => toggleComplete(chore)}
            />
          ))}
        </div>
      ) : (
        <div className="text-center py-20 bg-card/50 rounded-2xl border border-dashed border-border/60 backdrop-blur-sm">
          <div className="h-16 w-16 bg-primary/10 rounded-full flex items-center justify-center mx-auto mb-4">
            <ClipboardList className="h-8 w-8 text-primary" />
          </div>
          <h2 className="text-xl font-display font-semibold mb-2">No chores yet</h2>
          <p className="text-muted-foreground mb-6 max-w-md mx-auto">
            Add recurring household tasks and assign them to family members.
          </p>
          <Button onClick={openAdd}>
            <Plus className="h-4 w-4 mr-2" /> Create Your First Chore
          </Button>
        </div>
      )}

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="sm:max-w-[425px]">
          <DialogHeader>
            <DialogTitle>{editingChore ? 'Edit Chore' : 'New Chore'}</DialogTitle>
          </DialogHeader>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4 py-4">
            <div className="space-y-2">
              <Label htmlFor="title">Chore</Label>
              <Input id="title" placeholder="e.g. Take out the trash" {...form.register('title')} />
              {form.formState.errors.title && (
                <p className="text-xs text-destructive">{form.formState.errors.title.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="description">Description</Label>
              <Textarea id="description" placeholder="Details..." {...form.register('description')} className="resize-none" />
            </div>
            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label htmlFor="assignedTo">Assigned to</Label>
                <div className="relative">
                  <User className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
                  <Input id="assignedTo" placeholder="Name" className="pl-9" {...form.register('assignedTo')} />
                </div>
              </div>
              <div className="space-y-2">
                <Label htmlFor="dueDate">Due date</Label>
                <div className="relative">
                  <Calendar className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
                  <Input id="dueDate" type="date" className="pl-9" {...form.register('dueDate')} />
                </div>
              </div>
            </div>
            <div className="space-y-2">
              <Label>Frequency</Label>
              <div className="flex gap-4 flex-wrap">
                {FREQUENCIES.map((freq) => (
                  <label key={freq} className="flex items-center gap-2 text-sm capitalize cursor-pointer">
                    <input type="radio" value={freq} {...form.register('frequency')} className="accent-primary" />
                    {freq}
                  </label>
                ))}
              </div>
            </div>
            <DialogFooter className="pt-4">
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={form.formState.isSubmitting}>
                {form.formState.isSubmitting ? 'Saving...' : editingChore ? 'Save Changes' : 'Create Chore'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </PageTransition>
  );
}

function ChoreCard({
  chore,
  onEdit,
  onDelete,
  onToggle,
}: {
  chore: Chore;
  onEdit: () => void;
  onDelete: () => void;
  onToggle: () => void;
}) {
  const { formatDate } = useLocale();
  const isOverdue = !chore.completed && chore.dueDate && isPast(startOfDay(new Date(chore.dueDate)));
  return (
    <Card className={cn('group bg-card/70 hover:bg-accent border-border transition-colors', isOverdue && 'border-destructive/30')}>
      <CardContent className="p-5 flex items-start gap-4">
        <button
          onClick={onToggle}
          disabled={chore.completed}
          className={cn(
            'shrink-0 h-12 w-12 rounded-xl flex items-center justify-center border transition-colors',
            chore.completed
              ? 'bg-success/10 border-success/30 text-success'
              : 'bg-muted border-border text-muted-foreground hover:bg-accent hover:border-primary/30',
          )}
        >
          {chore.completed ? <CheckCircle2 className="h-6 w-6" /> : <div className="h-5 w-5 rounded-full border-2 border-current opacity-30" />}
        </button>
        <div className="flex-1 min-w-0">
          <h3 className={cn('font-medium truncate', chore.completed && 'line-through text-muted-foreground')}>
            {chore.title}
          </h3>
          {chore.description && <p className="text-xs text-muted-foreground truncate mt-0.5">{chore.description}</p>}
          <div className="flex flex-wrap items-center gap-3 mt-2 text-xs text-muted-foreground">
            <span className="flex items-center gap-1"><User className="h-3 w-3" /> {chore.assignedTo || 'Unassigned'}</span>
            <span className="flex items-center gap-1"><Repeat className="h-3 w-3" /> {chore.frequency}</span>
            {chore.dueDate && (
              <span className={cn('flex items-center gap-1', isOverdue && 'text-destructive')}>
                <Clock className="h-3 w-3" />
                {isOverdue ? 'Overdue ' : 'Due '}
                {formatDate(new Date(chore.dueDate), { month: 'short', day: 'numeric' })}
              </span>
            )}
          </div>
        </div>
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
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

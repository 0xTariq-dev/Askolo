import { useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  Plus,
  Trash2,
  Edit2,
  Save,
  CheckCircle2,
  Circle,
  Calendar,
  Zap,
  Headphones,
  SlidersHorizontal,
} from 'lucide-react';
import {
  useListActionItems,
  useCreateActionItem,
  useUpdateActionItem,
  useDeleteActionItem,
  getListActionItemsQueryKey,
  getGetDashboardSummaryQueryKey,
  ActionItem,
} from '@workspace/api-client-react';
import { useQueryClient } from '@tanstack/react-query';
import { PageTransition } from '@/components/ui/page-transition';
import { Card, CardContent, CardHeader, CardTitle } from '@workspace/askolo-design-system/components/ui/card';
import { Button } from '@workspace/askolo-design-system/components/ui/button';
import { Input } from '@workspace/askolo-design-system/components/ui/input';
import { Textarea } from '@workspace/askolo-design-system/components/ui/textarea';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@workspace/askolo-design-system/components/ui/dialog';
import { Label } from '@workspace/askolo-design-system/components/ui/label';
import { Badge } from '@workspace/askolo-design-system/components/ui/badge';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@workspace/askolo-design-system/components/ui/tabs';
import { cn } from '@workspace/askolo-design-system/lib/utils';

const actionSchema = z.object({
  title: z.string().min(1, 'Title is required'),
  dueDate: z.string().optional(),
});

type ActionForm = z.infer<typeof actionSchema>;

export function ActionsPage() {
  const qc = useQueryClient();
  const { data: items, isLoading } = useListActionItems();
  const createItem = useCreateActionItem();
  const updateItem = useUpdateActionItem();
  const deleteItem = useDeleteActionItem();

  const [dialogOpen, setDialogOpen] = useState(false);
  const [editingItem, setEditingItem] = useState<ActionItem | null>(null);
  const [filter, setFilter] = useState('open');

  const form = useForm<ActionForm>({
    resolver: zodResolver(actionSchema),
    defaultValues: { title: '', dueDate: '' },
  });

  const openAdd = () => {
    setEditingItem(null);
    form.reset({ title: '', dueDate: '' });
    setDialogOpen(true);
  };

  const openEdit = (item: ActionItem) => {
    setEditingItem(item);
    form.reset({ title: item.title, dueDate: item.dueDate || '' });
    setDialogOpen(true);
  };

  const onSubmit = (data: ActionForm) => {
    const payload = {
      title: data.title,
      dueDate: data.dueDate || undefined,
      sourceType: 'manual' as const,
    };

    if (editingItem) {
      updateItem.mutate(
        { id: editingItem.id, data: payload },
        {
          onSuccess: () => {
            qc.invalidateQueries({ queryKey: getListActionItemsQueryKey() });
            qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
            setDialogOpen(false);
          },
        },
      );
    } else {
      createItem.mutate(
        { data: payload },
        {
          onSuccess: () => {
            qc.invalidateQueries({ queryKey: getListActionItemsQueryKey() });
            qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
            setDialogOpen(false);
          },
        },
      );
    }
  };

  const toggleComplete = (item: ActionItem) => {
    const nextCompleted = !item.completed;
    updateItem.mutate(
      {
        id: item.id,
        data: { completed: nextCompleted },
      },
      {
        onSuccess: () => {
          qc.invalidateQueries({ queryKey: getListActionItemsQueryKey() });
          qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
        },
      },
    );
  };

  const handleDelete = (id: number) => {
    if (confirm('Delete this action item?')) {
      deleteItem.mutate(
        { id },
        {
          onSuccess: () => {
            qc.invalidateQueries({ queryKey: getListActionItemsQueryKey() });
            qc.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() });
          },
        },
      );
    }
  };

  const filteredItems = (items || [])
    .filter((item) => {
      if (filter === 'open') return !item.completed;
      if (filter === 'completed') return item.completed;
      return true;
    })
    .sort((a, b) => {
      if (a.completed !== b.completed) return a.completed ? 1 : -1;
      return (b.dueDate || '').localeCompare(a.dueDate || '');
    });

  const openCount = (items || []).filter((i) => !i.completed).length;
  const meetingCount = (items || []).filter((i) => i.sourceType === 'meeting' && !i.completed).length;
  const manualCount = (items || []).filter((i) => i.sourceType !== 'meeting' && !i.completed).length;

  return (
    <PageTransition className="max-w-5xl mx-auto space-y-6 pb-10">
      <header className="flex flex-col md:flex-row md:items-center justify-between gap-4">
        <div>
          <h1 className="text-3xl font-display font-bold tracking-tight">My Actions</h1>
          <p className="text-muted-foreground mt-1">
            {openCount} open · {meetingCount} from meetings · {manualCount} manual
          </p>
        </div>
        <Button onClick={openAdd} className="gap-2" data-testid="button-add-action">
          <Plus className="h-4 w-4" /> Add action
        </Button>
      </header>

      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <StatCard label="Open" value={openCount} icon={Circle} color="text-warning" bg="bg-warning/10" />
        <StatCard label="From meetings" value={meetingCount} icon={Headphones} color="text-info" bg="bg-info/10" />
        <StatCard label="Manual" value={manualCount} icon={Zap} color="text-success" bg="bg-success/10" />
      </div>

      <Tabs value={filter} onValueChange={setFilter} className="w-full">
        <TabsList className="bg-card border border-border">
          <TabsTrigger value="open" className="gap-2">
            <Circle className="h-4 w-4" /> Open
          </TabsTrigger>
          <TabsTrigger value="completed" className="gap-2">
            <CheckCircle2 className="h-4 w-4" /> Completed
          </TabsTrigger>
          <TabsTrigger value="all" className="gap-2">
            <SlidersHorizontal className="h-4 w-4" /> All
          </TabsTrigger>
        </TabsList>

        <TabsContent value={filter} className="mt-6">
          {isLoading ? (
            <div className="space-y-3">
              {[...Array(4)].map((_, i) => (
                <div key={i} className="h-20 bg-card rounded-xl animate-pulse" />
              ))}
            </div>
          ) : filteredItems.length > 0 ? (
            <div className="space-y-3">
              <AnimatePresence>
                {filteredItems.map((item) => (
                  <motion.div
                    key={item.id}
                    layout
                    initial={{ opacity: 0, y: 8 }}
                    animate={{ opacity: 1, y: 0 }}
                    exit={{ opacity: 0, y: -8 }}
                  >
                    <Card
                      className={cn(
                        'border-border bg-card/50 backdrop-blur-sm transition-colors group',
                        item.completed && 'opacity-60',
                      )}
                    >
                      <CardContent className="p-4 flex items-center gap-4">
                        <button
                          onClick={() => toggleComplete(item)}
                          className="shrink-0 focus:outline-none"
                          data-testid={`button-toggle-action-${item.id}`}
                        >
                          {item.completed ? (
                            <CheckCircle2 className="h-6 w-6 text-success" />
                          ) : (
                            <Circle className="h-6 w-6 text-muted-foreground/50 hover:text-primary transition-colors" />
                          )}
                        </button>

                        <div className="flex-1 min-w-0">
                          <div className="flex items-center gap-2 flex-wrap">
                            <p className={cn('font-medium', item.completed && 'line-through text-muted-foreground')}>
                              {item.title}
                            </p>
                            {item.sourceType === 'meeting' && (
                              <Badge variant="outline" className="text-xs gap-1">
                                <Headphones className="h-3 w-3" /> Meeting
                              </Badge>
                            )}
                          </div>
                          {item.dueDate && (
                            <p className="text-xs text-muted-foreground mt-1 flex items-center gap-1">
                              <Calendar className="h-3 w-3" /> Due {item.dueDate}
                            </p>
                          )}
                        </div>

                        <div className="flex gap-1 opacity-0 group-hover:opacity-100 transition-opacity">
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8"
                            onClick={() => openEdit(item)}
                            data-testid={`button-edit-action-${item.id}`}
                          >
                            <Edit2 className="h-4 w-4" />
                          </Button>
                          <Button
                            variant="ghost"
                            size="icon"
                            className="h-8 w-8 text-destructive hover:text-destructive"
                            onClick={() => handleDelete(item.id)}
                            data-testid={`button-delete-action-${item.id}`}
                          >
                            <Trash2 className="h-4 w-4" />
                          </Button>
                        </div>
                      </CardContent>
                    </Card>
                  </motion.div>
                ))}
              </AnimatePresence>
            </div>
          ) : (
            <Card className="border-dashed border-border bg-card/30">
              <CardContent className="py-16 text-center">
                <CheckCircle2 className="h-10 w-10 text-muted-foreground/50 mx-auto mb-4" />
                <p className="text-muted-foreground">
                  {filter === 'completed' ? 'No completed actions yet.' : 'No open action items. Great work!'}
                </p>
                {filter !== 'completed' && (
                  <Button variant="link" className="text-primary mt-2" onClick={openAdd}>
                    Add an action item
                  </Button>
                )}
              </CardContent>
            </Card>
          )}
        </TabsContent>
      </Tabs>

      <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
        <DialogContent className="bg-card border-border max-w-md">
          <DialogHeader>
            <DialogTitle className="font-display">
              {editingItem ? 'Edit action' : 'Add action'}
            </DialogTitle>
          </DialogHeader>
          <form onSubmit={form.handleSubmit(onSubmit)} className="space-y-4 py-2">
            <div className="space-y-2">
              <Label htmlFor="title">Title</Label>
              <Input id="title" {...form.register('title')} placeholder="What needs to get done?" />
              {form.formState.errors.title && (
                <p className="text-xs text-destructive">{form.formState.errors.title.message}</p>
              )}
            </div>
            <div className="space-y-2">
              <Label htmlFor="dueDate">Due date</Label>
              <Input id="dueDate" type="date" {...form.register('dueDate')} />
            </div>
            <DialogFooter>
              <Button type="button" variant="outline" onClick={() => setDialogOpen(false)}>
                Cancel
              </Button>
              <Button type="submit" disabled={createItem.isPending || updateItem.isPending}>
                <Save className="h-4 w-4 mr-2" />
                {editingItem ? 'Save changes' : 'Add action'}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </PageTransition>
  );
}

function StatCard({
  label,
  value,
  icon: Icon,
  color,
  bg,
}: {
  label: string;
  value: number;
  icon: React.ElementType;
  color: string;
  bg: string;
}) {
  return (
    <Card className="border-border bg-card/40 backdrop-blur-sm">
      <CardContent className="p-5 flex items-center justify-between">
        <div>
          <p className="text-sm text-muted-foreground">{label}</p>
          <p className="text-2xl font-display font-bold mt-1">{value}</p>
        </div>
        <div className={cn('h-10 w-10 rounded-xl flex items-center justify-center border border-border', bg, color)}>
          <Icon className="h-5 w-5" />
        </div>
      </CardContent>
    </Card>
  );
}

import { useEffect, useRef, useState } from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import {
  Send,
  Sparkles,
  User,
  Bot,
  X,
  Maximize2,
  Minimize2,
  MessageSquare,
} from 'lucide-react';
import {
  useAssistantChat,
  useGetDashboardSummary,
  useListGmailMessages,
} from '@workspace/api-client-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Form, FormControl, FormField, FormItem } from '@/components/ui/form';
import { cn } from '@/lib/utils';
import { useAssistantState, type ChatMessage } from '@/contexts/assistant-context';

const messageSchema = z.object({ text: z.string().min(1) });
type MessageForm = z.infer<typeof messageSchema>;

export function AssistantSidebar() {
  const { isOpen, isFull, messages, setMessages, toggle, close, toggleFull } =
    useAssistantState();
  const chat = useAssistantChat();
  const bottomRef = useRef<HTMLDivElement>(null);
  const [isThinking, setIsThinking] = useState(false);

  const { data: summary } = useGetDashboardSummary();
  const calendarConnected = summary?.googleConnection?.calendarConnected ?? false;
  const gmailConnected = summary?.googleConnection?.gmailConnected ?? false;

  // Fetch emails only when sidebar is open and Gmail is connected
  const { data: emailData } = useListGmailMessages({
    query: { enabled: isOpen && gmailConnected, staleTime: 5 * 60 * 1000, queryKey: ['listGmailMessages'] },
  });

  const form = useForm<MessageForm>({
    resolver: zodResolver(messageSchema),
    defaultValues: { text: '' },
  });

  useEffect(() => {
    if (isOpen) bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, isThinking, isOpen]);

  const onSubmit = async (data: MessageForm) => {
    const text = data.text.trim();
    if (!text || chat.isPending) return;

    const userMsg: ChatMessage = { role: 'user', content: text, id: `u-${Date.now()}` };
    const nextMessages = [...messages, userMsg];
    setMessages(nextMessages);
    form.reset();
    setIsThinking(true);

    const context = summary
      ? {
          habits: summary.habits.map((h) => ({
            id: h.id,
            name: h.name,
            currentStreak: h.currentStreak,
            longestStreak: h.longestStreak,
            completedToday: h.completedToday,
            color: h.color,
            icon: h.icon,
          })) as any,
          goals: summary.goals as any,
          todayPlan: summary.todayPlan.map((p) => ({
            title: p.title,
            completed: p.completed,
            priority: p.priority,
          })) as any,
          upcomingEvents: calendarConnected
            ? summary.upcomingEvents.map((e) => ({
                title: e.title,
                startDate: e.startDate,
                startTime: e.startTime ?? null,
                allDay: e.allDay,
              }))
            : [],
          recentEmails: gmailConnected
            ? (emailData?.messages ?? []).slice(0, 10).map((m) => ({
                subject: m.subject,
                from: m.from,
                priority: m.priority as string,
              }))
            : [],
        }
      : undefined;

    chat.mutate(
      { data: { messages: nextMessages.map((m) => ({ role: m.role, content: m.content })), context } },
      {
        onSuccess: (res) => {
          if (res?.message) {
            setMessages((prev) => [
              ...prev,
              { role: 'assistant', content: res.message, id: `a-${Date.now()}` },
            ]);
          }
        },
        onError: () => {
          setMessages((prev) => [
            ...prev,
            {
              role: 'assistant',
              content: 'Sorry, I had trouble responding. Please try again.',
              id: `e-${Date.now()}`,
            },
          ]);
        },
        onSettled: () => setIsThinking(false),
      },
    );
  };

  return (
    <>
      {/* Toggle button — bottom-right floating */}
      <button
        onClick={toggle}
        aria-label={isOpen ? 'Close assistant' : 'Open assistant'}
        className={cn(
          'fixed bottom-6 right-6 z-50 h-12 w-12 rounded-full shadow-lg flex items-center justify-center transition-colors',
          isOpen
            ? 'bg-primary/20 border border-primary/40 text-primary hover:bg-primary/30'
            : 'bg-primary text-primary-foreground hover:bg-primary/90',
        )}
      >
        {isOpen ? <X className="h-5 w-5" /> : <MessageSquare className="h-5 w-5" />}
      </button>

      {/* Overlay backdrop for fullscreen */}
      <AnimatePresence>
        {isOpen && isFull && (
          <motion.div
            key="backdrop"
            initial={{ opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            className="fixed inset-0 z-40 bg-black/50"
            onClick={close}
          />
        )}
      </AnimatePresence>

      {/* Sidebar panel */}
      <AnimatePresence>
        {isOpen && (
          <motion.aside
            key="sidebar"
            initial={{ x: '100%' }}
            animate={{ x: 0 }}
            exit={{ x: '100%' }}
            transition={{ type: 'spring', damping: 28, stiffness: 300 }}
            className={cn(
              'fixed top-0 right-0 bottom-0 z-50 flex flex-col bg-card border-l border-border shadow-2xl',
              isFull ? 'w-full' : 'w-[380px] max-w-full',
            )}
          >
            {/* Header */}
            <div className="h-14 flex items-center justify-between px-4 border-b border-border shrink-0">
              <div className="flex items-center gap-2">
                <div className="h-7 w-7 rounded-lg bg-primary/10 border border-primary/20 flex items-center justify-center text-primary">
                  <Sparkles className="h-3.5 w-3.5" />
                </div>
                <span className="font-display font-semibold text-sm">Askolo Assistant</span>
              </div>
              <div className="flex items-center gap-1">
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 text-muted-foreground hover:text-foreground"
                  onClick={toggleFull}
                  aria-label={isFull ? 'Collapse' : 'Expand to full screen'}
                >
                  {isFull ? <Minimize2 className="h-4 w-4" /> : <Maximize2 className="h-4 w-4" />}
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8 text-muted-foreground hover:text-foreground"
                  onClick={close}
                  aria-label="Close assistant"
                >
                  <X className="h-4 w-4" />
                </Button>
              </div>
            </div>

            {/* Messages */}
            <div className="flex-1 overflow-y-auto p-4 space-y-4">
              {messages.map((msg, idx) => (
                <motion.div
                  key={msg.id}
                  initial={{ opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: idx === 0 ? 0 : 0.04 }}
                  className={cn('flex gap-2.5', msg.role === 'user' ? 'flex-row-reverse' : 'flex-row')}
                >
                  <div
                    className={cn(
                      'h-7 w-7 rounded-full flex items-center justify-center shrink-0 border',
                      msg.role === 'user'
                        ? 'bg-primary/20 border-primary/30 text-primary'
                        : 'bg-white/10 border-white/10 text-foreground',
                    )}
                  >
                    {msg.role === 'user' ? <User className="h-3.5 w-3.5" /> : <Bot className="h-3.5 w-3.5" />}
                  </div>
                  <div
                    className={cn(
                      'max-w-[82%] rounded-2xl px-3.5 py-2.5 text-sm leading-relaxed',
                      msg.role === 'user'
                        ? 'bg-primary text-primary-foreground rounded-br-sm'
                        : 'bg-white/5 border border-white/10 text-foreground rounded-bl-sm',
                    )}
                  >
                    {msg.content}
                  </div>
                </motion.div>
              ))}
              {isThinking && (
                <motion.div
                  initial={{ opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  className="flex gap-2.5"
                >
                  <div className="h-7 w-7 rounded-full bg-white/10 border border-white/10 flex items-center justify-center">
                    <Bot className="h-3.5 w-3.5" />
                  </div>
                  <div className="bg-white/5 border border-white/10 rounded-2xl rounded-bl-sm px-3.5 py-2.5 flex items-center gap-1">
                    <span className="w-1.5 h-1.5 bg-muted-foreground rounded-full animate-bounce" />
                    <span className="w-1.5 h-1.5 bg-muted-foreground rounded-full animate-bounce [animation-delay:0.15s]" />
                    <span className="w-1.5 h-1.5 bg-muted-foreground rounded-full animate-bounce [animation-delay:0.3s]" />
                  </div>
                </motion.div>
              )}
              <div ref={bottomRef} />
            </div>

            {/* Input */}
            <div className="p-3 border-t border-border bg-card/50 shrink-0">
              <Form {...form}>
                <form onSubmit={form.handleSubmit(onSubmit)} className="flex gap-2">
                  <FormField
                    control={form.control}
                    name="text"
                    render={({ field }) => (
                      <FormItem className="flex-1">
                        <FormControl>
                          <Input
                            placeholder="Ask Askolo something…"
                            className="h-10 bg-background border-border text-sm"
                            {...field}
                            onKeyDown={(e) => {
                              if (e.key === 'Enter' && !e.shiftKey) {
                                e.preventDefault();
                                form.handleSubmit(onSubmit)();
                              }
                            }}
                          />
                        </FormControl>
                      </FormItem>
                    )}
                  />
                  <Button
                    type="submit"
                    size="icon"
                    className="h-10 w-10 shrink-0"
                    disabled={chat.isPending || isThinking}
                  >
                    <Send className="h-4 w-4" />
                  </Button>
                </form>
              </Form>
            </div>
          </motion.aside>
        )}
      </AnimatePresence>
    </>
  );
}

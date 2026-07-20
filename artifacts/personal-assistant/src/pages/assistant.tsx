import { useEffect, useRef, useState } from 'react';
import { motion } from 'framer-motion';
import { useForm } from 'react-hook-form';
import { zodResolver } from '@hookform/resolvers/zod';
import { z } from 'zod';
import { Send, Sparkles, User, Bot } from 'lucide-react';
import { useAssistantChat, useGetDashboardSummary, AssistantChatContext } from '@workspace/api-client-react';
import { PageTransition } from '@/components/ui/page-transition';
import { Card, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Form, FormControl, FormField, FormItem } from '@/components/ui/form';
import { cn } from '@/lib/utils';
import { useQueryClient } from '@tanstack/react-query';

const messageSchema = z.object({
  text: z.string().min(1),
});

type MessageForm = z.infer<typeof messageSchema>;

interface ChatMessage {
  role: 'user' | 'assistant';
  content: string;
  id: string;
}

export function AssistantPage() {
  const qc = useQueryClient();
  const { data: summary } = useGetDashboardSummary();
  const chat = useAssistantChat();
  const bottomRef = useRef<HTMLDivElement>(null);
  const [messages, setMessages] = useState<ChatMessage[]>([
    {
      role: 'assistant',
      content: "Hi, I'm Aura. Ask me about your day, what to focus on, or how to balance your goals.",
      id: 'welcome',
    },
  ]);
  const [isThinking, setIsThinking] = useState(false);

  const form = useForm<MessageForm>({
    resolver: zodResolver(messageSchema),
    defaultValues: { text: '' },
  });

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' });
  }, [messages, isThinking]);

  const onSubmit = async (data: MessageForm) => {
    const text = data.text.trim();
    if (!text || chat.isPending) return;

    const userMsg: ChatMessage = { role: 'user', content: text, id: `u-${Date.now()}` };
    const nextMessages = [...messages, userMsg];
    setMessages(nextMessages);
    form.reset();
    setIsThinking(true);

    const context: AssistantChatContext | undefined = summary
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
        }
      : undefined;

    chat.mutate(
      {
        data: {
          messages: nextMessages.map((m) => ({ role: m.role, content: m.content })),
          context,
        },
      },
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
            { role: 'assistant', content: 'Sorry, I had trouble responding. Please try again.', id: `e-${Date.now()}` },
          ]);
        },
        onSettled: () => setIsThinking(false),
      },
    );
  };

  return (
    <PageTransition className="h-[calc(100vh-8rem)] flex flex-col max-w-4xl mx-auto">
      <header className="mb-6">
        <div className="flex items-center gap-3">
          <div className="h-10 w-10 rounded-xl bg-primary/10 border border-primary/20 flex items-center justify-center text-primary">
            <Sparkles className="h-5 w-5" />
          </div>
          <div>
            <h1 className="text-2xl font-display font-bold tracking-tight">Aura Assistant</h1>
            <p className="text-sm text-muted-foreground">Ask anything about your habits, goals, or day.</p>
          </div>
        </div>
      </header>

      <Card className="flex-1 flex flex-col border-border bg-card/50 backdrop-blur-sm overflow-hidden">
        <CardContent className="flex-1 flex flex-col p-0">
          <div className="flex-1 overflow-y-auto p-4 md:p-6 space-y-5">
            {messages.map((msg, idx) => (
              <motion.div
                key={msg.id}
                initial={{ opacity: 0, y: 10 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: idx === 0 ? 0 : 0.05 }}
                className={cn('flex gap-3', msg.role === 'user' ? 'flex-row-reverse' : 'flex-row')}
              >
                <div
                  className={cn(
                    'h-8 w-8 rounded-full flex items-center justify-center shrink-0 border',
                    msg.role === 'user'
                      ? 'bg-primary/20 border-primary/30 text-primary'
                      : 'bg-white/10 border-white/10 text-foreground',
                  )}
                >
                  {msg.role === 'user' ? <User className="h-4 w-4" /> : <Bot className="h-4 w-4" />}
                </div>
                <div
                  className={cn(
                    'max-w-[80%] rounded-2xl px-4 py-3 text-sm leading-relaxed',
                    msg.role === 'user'
                      ? 'bg-primary text-primary-foreground rounded-br-md'
                      : 'bg-white/5 border border-white/10 text-foreground rounded-bl-md',
                  )}
                >
                  {msg.content}
                </div>
              </motion.div>
            ))}
            {isThinking && (
              <motion.div
                initial={{ opacity: 0, y: 10 }}
                animate={{ opacity: 1, y: 0 }}
                className="flex gap-3"
              >
                <div className="h-8 w-8 rounded-full bg-white/10 border border-white/10 flex items-center justify-center">
                  <Bot className="h-4 w-4" />
                </div>
                <div className="bg-white/5 border border-white/10 rounded-2xl rounded-bl-md px-4 py-3 flex items-center gap-1">
                  <span className="w-2 h-2 bg-muted-foreground rounded-full animate-bounce" />
                  <span className="w-2 h-2 bg-muted-foreground rounded-full animate-bounce [animation-delay:0.15s]" />
                  <span className="w-2 h-2 bg-muted-foreground rounded-full animate-bounce [animation-delay:0.3s]" />
                </div>
              </motion.div>
            )}
            <div ref={bottomRef} />
          </div>

          <div className="p-4 border-t border-border bg-card/30">
            <Form {...form}>
              <form onSubmit={form.handleSubmit(onSubmit)} className="flex gap-3">
                <FormField
                  control={form.control}
                  name="text"
                  render={({ field }) => (
                    <FormItem className="flex-1">
                      <FormControl>
                        <Input
                          placeholder="Ask Aura something..."
                          className="h-12 bg-background border-border"
                          {...field}
                          data-testid="assistant-input"
                        />
                      </FormControl>
                    </FormItem>
                  )}
                />
                <Button
                  type="submit"
                  size="icon"
                  className="h-12 w-12 shrink-0"
                  disabled={chat.isPending || isThinking}
                  data-testid="assistant-send"
                >
                  <Send className="h-5 w-5" />
                </Button>
              </form>
            </Form>
          </div>
        </CardContent>
      </Card>
    </PageTransition>
  );
}

import { useEffect, useRef, useState } from 'react';
import { motion, AnimatePresence, useReducedMotion } from 'framer-motion';
import { useForm } from 'react-hook-form';
import { useQueryClient } from '@tanstack/react-query';
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
  Check,
  Loader2,
} from 'lucide-react';
import {
  confirmAssistantRun,
  cancelAssistantRun,
  useGetAssistantConversation,
  getGetAssistantConversationQueryKey,
  getGetDashboardSummaryQueryKey,
  getListActionItemsQueryKey,
  type AssistantRun,
} from '@workspace/api-client-react';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Form, FormControl, FormField, FormItem } from '@/components/ui/form';
import { cn } from '@/lib/utils';
import { useAssistantState, type ChatMessage } from '@/contexts/assistant-context';
import { createAssistantRunOverWebSocket } from '@/lib/assistant-run-websocket';
import { creditApi, creditErrorMessage, newCreditIdempotencyKey } from '@/lib/credit-api';

const messageSchema = z.object({ text: z.string().min(1) });
type MessageForm = z.infer<typeof messageSchema>;

export function AssistantSidebar() {
  const {
    isOpen,
    isFull,
    messages,
    setMessages,
    toggle,
    close,
    toggleFull,
    draft,
    clearDraft,
  } = useAssistantState();
  const queryClient = useQueryClient();
  const bottomRef = useRef<HTMLDivElement>(null);
  const planningRequestRef = useRef<AbortController | null>(null);
  const reducedMotion = useReducedMotion();
  const [isThinking, setIsThinking] = useState(false);
  const [pendingActionId, setPendingActionId] = useState<string | null>(null);
  const [assistantError, setAssistantError] = useState('');
  const conversationQuery = useGetAssistantConversation({
    query: {
      queryKey: getGetAssistantConversationQueryKey(),
      enabled: isOpen,
      staleTime: 0,
      refetchOnWindowFocus: true,
    },
  });

  const form = useForm<MessageForm>({
    resolver: zodResolver(messageSchema),
    defaultValues: { text: '' },
  });

  useEffect(() => {
    if (isOpen) bottomRef.current?.scrollIntoView({ behavior: reducedMotion ? 'auto' : 'smooth' });
  }, [messages, isThinking, isOpen, reducedMotion]);

  useEffect(() => () => planningRequestRef.current?.abort(), []);

  useEffect(() => {
    if (!isOpen) return;
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') close();
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [close, isOpen]);

  useEffect(() => {
    if (!conversationQuery.data) return;
    setMessages(
      conversationQuery.data.messages.length > 0
        ? conversationQuery.data.messages
        : [{
            role: 'assistant',
            content: "Hi, I'm Askolo. Ask me to prepare one action item for your list.",
            id: 'welcome',
          }],
    );
  }, [conversationQuery.data, setMessages]);

  useEffect(() => {
    if (!draft) return;
    form.reset({ text: draft });
    clearDraft();
  }, [draft, clearDraft, form]);

  const incorporateRun = (run: AssistantRun) => {
    setMessages((previous) => {
      const updated = previous.map((message) =>
        message.runId === run.id
          ? {
              ...message,
              state: run.state,
              intent: run.intent ?? message.intent,
              intentSha256: run.intentSha256 ?? message.intentSha256,
              requiresConfirmation: run.requiresConfirmation,
              confirmationExpiresAt: run.confirmationExpiresAt ?? message.confirmationExpiresAt,
              result: run.result ?? message.result,
            }
          : message,
      );
      if (!run.message || updated.some((message) => message.runId === run.id && message.content === run.message)) {
        return updated;
      }
      return [
        ...updated,
        {
          id: `run-${run.id}-${run.updatedAt}`,
          role: 'assistant',
          content: run.message,
          runId: run.id,
          state: run.state,
          intent: run.intent,
          intentSha256: run.intentSha256,
          requiresConfirmation: run.requiresConfirmation,
          confirmationExpiresAt: run.confirmationExpiresAt,
          result: run.result,
        },
      ];
    });
  };

  const onSubmit = async (data: MessageForm) => {
    const text = data.text.trim();
    if (!text || isThinking) return;

    setMessages((previous) => [
      ...previous,
      { role: 'user', content: text, id: `local-${Date.now()}` },
    ]);
    form.reset();
    setIsThinking(true);
    setAssistantError('');
    const controller = new AbortController();
    planningRequestRef.current = controller;

    try {
      const estimate = await creditApi.estimate('assistant', 1);
      if (controller.signal.aborted) {
        setAssistantError('Request stopped. No action was taken.');
        await queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() });
        return;
      }
      if (!estimate.canReserve) {
        throw new Error('There are not enough AI credits for this request.');
      }
      const run = await createAssistantRunOverWebSocket({
        conversationId: conversationQuery.data?.conversationId,
        transcript: text,
        idempotencyKey: newCreditIdempotencyKey(),
        policyVersion: estimate.policyVersion,
      }, controller.signal);
      incorporateRun(run);
      await queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() });
    } catch (error) {
      if (controller.signal.aborted) {
        setAssistantError('Request stopped. No action was taken.');
        await queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() });
      } else {
        setAssistantError(
          creditErrorMessage(error, 'I could not complete that request. Please try again.'),
        );
        await queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() });
      }
    } finally {
      if (planningRequestRef.current === controller) planningRequestRef.current = null;
      setIsThinking(false);
    }
  };

  const handleConfirm = async (message: ChatMessage) => {
    if (!message.runId || !message.intentSha256 || pendingActionId) return;
    setPendingActionId(message.runId);
    setAssistantError('');
    try {
      const run = await confirmAssistantRun(message.runId, {
        expectedIntentSHA256: message.intentSha256,
      });
      incorporateRun(run);
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() }),
        queryClient.invalidateQueries({ queryKey: getListActionItemsQueryKey() }),
        queryClient.invalidateQueries({ queryKey: getGetDashboardSummaryQueryKey() }),
      ]);
    } catch (error) {
      setAssistantError(
        creditErrorMessage(error, 'That action could not be confirmed. Review it and try again.'),
      );
      await queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() });
    } finally {
      setPendingActionId(null);
    }
  };

  const handleCancel = async (message: ChatMessage) => {
    if (!message.runId || !message.intentSha256 || pendingActionId) return;
    setPendingActionId(message.runId);
    setAssistantError('');
    try {
      const run = await cancelAssistantRun(message.runId, {
        expectedIntentSHA256: message.intentSha256,
      });
      incorporateRun(run);
      await queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() });
    } catch (error) {
      setAssistantError(
        creditErrorMessage(error, 'That pending action could not be dismissed. Try again.'),
      );
      await queryClient.invalidateQueries({ queryKey: getGetAssistantConversationQueryKey() });
    } finally {
      setPendingActionId(null);
    }
  };

  return (
    <>
      {/* Toggle button — bottom-right floating */}
      <button
        onClick={toggle}
        aria-label={isOpen ? 'Close assistant' : 'Open assistant'}
        aria-expanded={isOpen}
        aria-controls="assistant-panel"
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
            initial={reducedMotion ? false : { opacity: 0 }}
            animate={{ opacity: 1 }}
            exit={{ opacity: 0 }}
            transition={reducedMotion ? { duration: 0 } : undefined}
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
            id="assistant-panel"
            aria-label="Askolo assistant"
            initial={reducedMotion ? { x: 0, opacity: 1 } : { x: '100%' }}
            animate={{ x: 0 }}
            exit={reducedMotion ? { opacity: 0 } : { x: '100%' }}
            transition={reducedMotion ? { duration: 0 } : { type: 'spring', damping: 28, stiffness: 300 }}
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
                  className="h-10 w-10 text-muted-foreground hover:text-foreground"
                  onClick={toggleFull}
                  aria-label={isFull ? 'Collapse' : 'Expand to full screen'}
                >
                  {isFull ? <Minimize2 className="h-4 w-4" /> : <Maximize2 className="h-4 w-4" />}
                </Button>
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-10 w-10 text-muted-foreground hover:text-foreground"
                  onClick={close}
                  aria-label="Close assistant"
                >
                  <X className="h-4 w-4" />
                </Button>
              </div>
            </div>

            {/* Messages */}
             <div
               className="flex-1 overflow-y-auto p-4 space-y-4"
               role="log"
               aria-label="Assistant conversation"
               aria-live="polite"
               aria-relevant="additions text"
             >
              {conversationQuery.isError && (
                <p className="text-xs text-destructive" role="status">
                  Saved assistant history could not be loaded. New messages can still be sent.
                </p>
              )}
              {messages.map((msg, idx) => (
                <motion.div
                  key={msg.id}
                  initial={reducedMotion ? false : { opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={reducedMotion ? { duration: 0 } : { delay: idx === 0 ? 0 : 0.04 }}
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
                    {msg.role === 'assistant' &&
                      msg.state === 'needs_confirmation' &&
                      msg.intent?.tool === 'create_action_item' &&
                      msg.runId &&
                      msg.intentSha256 && (
                        <div className="mt-3 rounded-lg border border-primary/30 bg-primary/5 p-3 space-y-2">
                          <p className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
                            Action item to add
                          </p>
                          <p className="text-sm font-medium break-words">{msg.intent.title}</p>
                          {msg.confirmationExpiresAt &&
                          new Date(msg.confirmationExpiresAt).getTime() > Date.now() ? (
                            <div className="flex flex-wrap gap-2 pt-1">
                              <Button
                                type="button"
                                size="sm"
                                className="min-h-10"
                                disabled={pendingActionId !== null || isThinking}
                                onClick={() => void handleConfirm(msg)}
                                aria-label={`Confirm adding ${msg.intent.title}`}
                              >
                                {pendingActionId === msg.runId ? (
                                  <Loader2 className="h-4 w-4 mr-1 animate-spin" />
                                ) : (
                                  <Check className="h-4 w-4 mr-1" />
                                )}
                                Confirm add
                              </Button>
                              <Button
                                type="button"
                                size="sm"
                                variant="outline"
                                className="min-h-10"
                                disabled={pendingActionId !== null || isThinking}
                                onClick={() => void handleCancel(msg)}
                                aria-label={`Dismiss adding ${msg.intent.title}`}
                              >
                                Not now
                              </Button>
                            </div>
                          ) : (
                            <p className="text-xs text-muted-foreground" role="status">
                              This confirmation has expired. Send the request again to review a new action.
                            </p>
                          )}
                        </div>
                      )}
                  </div>
                </motion.div>
              ))}
              {isThinking && (
                <motion.div
                  initial={reducedMotion ? false : { opacity: 0, y: 8 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={reducedMotion ? { duration: 0 } : undefined}
                  className="flex gap-2.5"
                >
                  <div className="h-7 w-7 rounded-full bg-white/10 border border-white/10 flex items-center justify-center">
                    <Bot className="h-3.5 w-3.5" />
                  </div>
                  <div className="bg-white/5 border border-white/10 rounded-2xl rounded-bl-sm px-3.5 py-2.5 flex items-center gap-1">
                    <span className="w-1.5 h-1.5 bg-muted-foreground rounded-full animate-bounce motion-reduce:animate-none" />
                    <span className="w-1.5 h-1.5 bg-muted-foreground rounded-full animate-bounce motion-reduce:animate-none [animation-delay:0.15s]" />
                    <span className="w-1.5 h-1.5 bg-muted-foreground rounded-full animate-bounce motion-reduce:animate-none [animation-delay:0.3s]" />
                  </div>
                  <Button
                    type="button"
                    size="sm"
                    variant="ghost"
                    className="self-center min-h-10"
                    onClick={() => planningRequestRef.current?.abort()}
                  >
                    Stop
                  </Button>
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
                            aria-label="Message Askolo"
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
                    disabled={isThinking || pendingActionId !== null}
                    aria-label="Send message"
                  >
                    <Send className="h-4 w-4" />
                  </Button>
                </form>
              </Form>
              {assistantError && (
                <p className="mt-2 px-1 text-xs text-destructive" role="alert">
                  {assistantError}
                </p>
              )}
              <p className="mt-2 px-1 text-[10px] leading-relaxed text-muted-foreground">
                Messages you send are saved to your account. Actions are only added after you confirm.
              </p>
            </div>
          </motion.aside>
        )}
      </AnimatePresence>
    </>
  );
}

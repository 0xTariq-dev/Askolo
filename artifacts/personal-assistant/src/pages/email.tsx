import { useState, useMemo, useEffect } from 'react';
import { useQueryClient } from '@tanstack/react-query';
import { useLocation } from 'wouter';
import {
  Mail,
  RefreshCw,
  Reply,
  Send,
  X,
  AlertCircle,
  Clock,
  CheckCircle2,
  Inbox,
  ArrowRight,
} from 'lucide-react';
import {
  useListGmailMessages,
  useGenerateGmailDraft,
  useSendGmailMessage,
  useGetGoogleStatus,
  getListGmailMessagesQueryKey,
  type GmailMessage,
} from '@workspace/api-client-react';
import { PageTransition } from '@/components/ui/page-transition';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog';
import { Textarea } from '@/components/ui/textarea';
import { Label } from '@/components/ui/label';
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/utils';
import logoUrl from '/logo.png';
import { useLocale } from '@/contexts/locale-context';

const PRIORITY_ORDER = ['urgent', 'follow-up', 'fyi', 'archive'] as const;
const PRIORITY_LABELS: Record<string, string> = {
  urgent: 'Urgent',
  'follow-up': 'Follow-up',
  fyi: 'FYI',
  archive: 'Can Archive',
};

const GOOGLE_CONNECT_URL = '/api/google/gmail/connect?redirectTo=%2Femail&scope=gmail';

function EmailPageHeader({
  connected,
  isLoading,
  onRefresh,
}: {
  connected: boolean;
  isLoading?: boolean;
  onRefresh?: () => void;
}) {
  return (
    <header
      className={
        connected
          ? 'flex flex-col sm:flex-row sm:items-end justify-between gap-4 mb-8'
          : 'mb-8'
      }
    >
      <div>
        <h1 className="text-3xl md:text-4xl font-display font-bold tracking-tight">Email Triage</h1>
        <p className="text-muted-foreground mt-2 text-lg">
          {connected
            ? 'AI-prioritized inbox and one-click reply drafts.'
            : 'AI-powered inbox prioritization and reply drafting.'}
        </p>
      </div>
      {connected && onRefresh ? (
        <Button variant="outline" onClick={onRefresh} disabled={isLoading} className="shrink-0">
          <RefreshCw className={cn('h-4 w-4 mr-2', isLoading && 'animate-spin')} />
          Refresh
        </Button>
      ) : null}
    </header>
  );
}

export function EmailPage() {
  const qc = useQueryClient();
  const [, setLocation] = useLocation();
  const { toast } = useToast();
  const [filter, setFilter] = useState('all');
  const [selectedMessage, setSelectedMessage] = useState<GmailMessage | null>(null);
  const [draft, setDraft] = useState('');
  const [tone, setTone] = useState('professional');
  const [replyOpen, setReplyOpen] = useState(false);

  const { data: googleStatus, refetch: refetchGoogleStatus, isFetching: isCheckingStatus } = useGetGoogleStatus();
  const { data: messagesData, isLoading, error, refetch } = useListGmailMessages();
  const generateDraft = useGenerateGmailDraft();
  const sendMessage = useSendGmailMessage();

  const messages = messagesData?.messages || [];

  useEffect(() => {
    const params = new URLSearchParams(window.location.search);
    if (params.get('google') === 'connected') {
      toast({ title: 'Google connected', description: 'Gmail and Calendar access is now enabled.' });
      refetchGoogleStatus();
      refetch();
      setLocation('/email', { replace: true });
    } else if (params.get('google') === 'error') {
      toast({ variant: 'destructive', title: 'Google connection failed', description: 'Please try connecting again.' });
      setLocation('/email', { replace: true });
    }
  }, [toast, refetchGoogleStatus, refetch, setLocation]);

  const filtered = useMemo(() => {
    if (filter === 'all') return messages;
    return messages.filter((m) => m.priority === filter);
  }, [messages, filter]);

  const counts = useMemo(() => {
    return {
      all: messages.length,
      urgent: messages.filter((m) => m.priority === 'urgent').length,
      'follow-up': messages.filter((m) => m.priority === 'follow-up').length,
      fyi: messages.filter((m) => m.priority === 'fyi').length,
      archive: messages.filter((m) => m.priority === 'archive').length,
    };
  }, [messages]);

  const openReply = (message: GmailMessage) => {
    setSelectedMessage(message);
    setDraft('');
    setReplyOpen(true);
    generateDraft.mutate(
      { data: { messageId: message.id, tone } },
      {
        onSuccess: (data) => {
          setDraft(data.draft || '');
        },
      },
    );
  };

  const handleSend = () => {
    if (!selectedMessage || !draft.trim()) return;
    sendMessage.mutate(
      {
        data: {
          to: selectedMessage.from,
          subject: selectedMessage.subject || '',
          body: draft,
          threadId: selectedMessage.threadId,
        },
      },
      {
        onSuccess: () => {
          setReplyOpen(false);
          setSelectedMessage(null);
          setDraft('');
          qc.invalidateQueries({ queryKey: getListGmailMessagesQueryKey() });
        },
      },
    );
  };

  if (!googleStatus?.gmailConnected || error) {
    return (
      <PageTransition surface={false} className="max-w-5xl mx-auto pb-10">
        <EmailPageHeader connected={false} />

        <Card className="border border-border/60 bg-card/50 backdrop-blur-sm">
          <CardContent className="p-8 sm:p-12 text-center">
            <div className="mx-auto h-20 w-20 rounded-2xl bg-primary/10 border border-border flex items-center justify-center mb-6 shadow-sm">
              <img src={logoUrl} alt="Askolo" className="h-12 w-12 object-contain" />
            </div>
            <h2 className="text-2xl font-display font-semibold mb-3">Connect Gmail</h2>
            <p className="text-muted-foreground max-w-md mx-auto mb-6">
              Askolo needs access to your Gmail inbox to triage messages and draft replies. The connection uses your{' '}
              <span className="text-foreground font-medium">Askolo</span> Google app, so the consent screen shows the
              correct name and logo.
            </p>
            <div className="flex flex-col sm:flex-row items-center justify-center gap-3">
              <Button size="lg" onClick={() => (window.location.href = GOOGLE_CONNECT_URL)} className="group">
                <Mail className="mr-2 h-5 w-5" />
                Connect Gmail
                <ArrowRight className="ml-2 h-4 w-4 opacity-70 group-hover:translate-x-0.5 transition-transform" />
              </Button>
              <Button variant="outline" size="lg" onClick={() => refetchGoogleStatus()} disabled={isCheckingStatus}>
                <RefreshCw className={cn('h-4 w-4 mr-2', isCheckingStatus && 'animate-spin')} />
                Check connection
              </Button>
            </div>
            <p className="text-xs text-muted-foreground mt-6">
              Make sure your Google Cloud OAuth consent screen is named "Askolo" and the redirect URI is added before
              connecting.
            </p>
          </CardContent>
        </Card>
      </PageTransition>
    );
  }

  return (
    <PageTransition surface={false} className="max-w-5xl mx-auto pb-10">
      <EmailPageHeader connected isLoading={isLoading} onRefresh={() => refetch()} />

      <Tabs value={filter} onValueChange={setFilter} className="w-full mb-6">
        <TabsList className="bg-card border border-border flex-wrap h-auto py-1">
          {PRIORITY_ORDER.map((p) => (
            <TabsTrigger key={p} value={p} className="capitalize">
              {PRIORITY_LABELS[p]} ({counts[p]})
            </TabsTrigger>
          ))}
          <TabsTrigger value="all">All ({counts.all})</TabsTrigger>
        </TabsList>
      </Tabs>

      {isLoading ? (
        <div className="space-y-3">
          {[...Array(4)].map((_, i) => (
            <div key={i} className="h-24 bg-card rounded-xl animate-pulse" />
          ))}
        </div>
      ) : filtered.length > 0 ? (
        <div className="space-y-3">
          {filtered.map((message) => (
            <EmailCard
              key={message.id}
              message={message}
              onReply={() => openReply(message)}
            />
          ))}
        </div>
      ) : (
        <div className="text-center py-20 bg-card/50 rounded-2xl border border-dashed border-border/60 backdrop-blur-sm">
          <div className="h-16 w-16 bg-primary/10 rounded-full flex items-center justify-center mx-auto mb-4">
            <Inbox className="h-8 w-8 text-primary" />
          </div>
          <h2 className="text-xl font-display font-semibold mb-2">Inbox empty</h2>
          <p className="text-muted-foreground">No messages matched the selected filter.</p>
        </div>
      )}

      <Dialog open={replyOpen} onOpenChange={setReplyOpen}>
        <DialogContent className="sm:max-w-[640px]">
          <DialogHeader>
            <DialogTitle className="flex items-center gap-2">
              <Reply className="h-5 w-5 text-primary" />
              Reply to {selectedMessage?.from}
            </DialogTitle>
          </DialogHeader>
          <div className="space-y-4 py-4">
            <div>
              <Label className="text-xs text-muted-foreground">Subject</Label>
              <p className="text-sm font-medium">{selectedMessage?.subject}</p>
            </div>
            <div>
              <Label className="text-xs text-muted-foreground">Tone</Label>
              <div className="flex gap-2 mt-1">
                {['professional', 'friendly', 'brief'].map((t) => (
                  <button
                    key={t}
                    type="button"
                    onClick={() => {
                      setTone(t);
                      if (selectedMessage) {
                        generateDraft.mutate({ data: { messageId: selectedMessage.id, tone: t } }, {
                          onSuccess: (data) => setDraft(data.draft || ''),
                        });
                      }
                    }}
                    className={cn(
                      'px-3 py-1 rounded-md text-xs border capitalize',
                      tone === t
                        ? 'bg-primary/20 border-primary/30 text-primary'
                        : 'border-border hover:bg-accent'
                    )}
                  >
                    {t}
                  </button>
                ))}
              </div>
            </div>
            <div>
              <Label htmlFor="draft">Draft reply</Label>
              <Textarea
                id="draft"
                value={draft}
                onChange={(e) => setDraft(e.target.value)}
                placeholder="AI is drafting a reply..."
                className="min-h-[160px] resize-none mt-1"
              />
            </div>
          </div>
          <DialogFooter className="gap-2">
            <Button variant="outline" onClick={() => setReplyOpen(false)}>
              <X className="h-4 w-4 mr-2" /> Cancel
            </Button>
            <Button
              onClick={handleSend}
              disabled={sendMessage.isPending || !draft.trim()}
            >
              {sendMessage.isPending ? 'Sending...' : <><Send className="h-4 w-4 mr-2" /> Send</>}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </PageTransition>
  );
}

function EmailCard({
  message,
  onReply,
}: {
  message: GmailMessage;
  onReply: () => void;
}) {
  const { formatDate } = useLocale();
  const priorityColor = {
    urgent: 'bg-destructive/10 text-destructive border-destructive/20',
    'follow-up': 'bg-warning/10 text-warning border-warning/20',
    fyi: 'bg-info/10 text-info border-info/20',
    archive: 'bg-muted text-muted-foreground border-border',
  }[message.priority] || 'bg-info/10 text-info border-info/20';

  const priorityIcon = {
    urgent: AlertCircle,
    'follow-up': Clock,
    fyi: CheckCircle2,
    archive: CheckCircle2,
  }[message.priority] || CheckCircle2;

  const Icon = priorityIcon;

  return (
    <Card className="group bg-card/40 hover:bg-card/60 border-border transition-colors">
      <CardContent className="p-4 flex items-start gap-4">
        <div className={cn('h-10 w-10 rounded-full flex items-center justify-center border shrink-0', priorityColor)}>
          <Icon className="h-5 w-5" />
        </div>
        <div className="flex-1 min-w-0">
          <div className="flex flex-col sm:flex-row sm:items-center gap-1 sm:gap-3">
            <h3 className="font-medium truncate">{message.subject || '(No subject)'}</h3>
            <span className={cn('text-xs px-2 py-0.5 rounded border w-fit', priorityColor)}>
              {PRIORITY_LABELS[message.priority] || message.priority}
            </span>
          </div>
          <p className="text-xs text-muted-foreground mt-1">{message.from}</p>
          <p className="text-sm text-foreground/80 mt-2 line-clamp-2">{message.snippet || ''}</p>
          <div className="flex items-center justify-between mt-3">
            <span className="text-xs text-muted-foreground">
              {message.internalDate ? formatDate(Number(message.internalDate), { month: 'short', day: 'numeric', hour: 'numeric', minute: '2-digit' }) : ''}
            </span>
            <Button size="sm" variant="outline" onClick={onReply} className="border-border">
              <Reply className="h-4 w-4 me-2" /> Draft reply
            </Button>
          </div>
        </div>
      </CardContent>
    </Card>
  );
}

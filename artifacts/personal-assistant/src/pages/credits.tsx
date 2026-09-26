import { useEffect, useMemo, useState } from 'react';
import { ArrowDownLeft, ArrowUpRight, Check, Clipboard, Coins, Download, History, Loader2, RefreshCw, ShieldCheck, Sparkles, WalletCards } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { PageTransition } from '@/components/ui/page-transition';
import { Skeleton } from '@/components/ui/skeleton';
import { useToast } from '@/hooks/use-toast';
import { creditApi, creditErrorMessage, type CreditAccountResponse, type CreditReceipt, type CreditUsageResponse } from '@/lib/credit-api';

function formatDate(value?: string | null) {
  if (!value) return '—';
  return new Intl.DateTimeFormat(undefined, { month: 'short', day: 'numeric', year: 'numeric', hour: 'numeric', minute: '2-digit' }).format(new Date(value));
}

function ReceiptRow({ receipt, onOpen }: { receipt: CreditReceipt; onOpen: () => void }) {
  const isEvent = Boolean(receipt.eventType);
  const amount = isEvent ? receipt.credits ?? 0 : receipt.settledCredits ?? receipt.reservedCredits ?? 0;
  const positive = isEvent && receipt.eventType === 'refunded';
  return (
    <button type="button" onClick={onOpen} data-testid={`button-open-receipt-${receipt.id}`} className="group flex w-full items-center gap-3 rounded-xl border border-border/70 bg-background/35 p-3 text-left transition-colors hover:border-primary/40 hover:bg-primary/5">
      <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border ${positive ? 'border-emerald-500/30 bg-emerald-500/10 text-emerald-500' : 'border-primary/25 bg-primary/10 text-primary'}`}>
        {positive ? <ArrowDownLeft className="h-4 w-4" /> : <ArrowUpRight className="h-4 w-4" />}
      </span>
      <span className="min-w-0 flex-1">
        <span className="block truncate text-sm font-medium">{isEvent ? receipt.eventType : `${receipt.operationType ?? 'AI'} operation`}</span>
        <span className="block truncate text-xs text-muted-foreground">{formatDate(receipt.createdAt)} · {isEvent ? receipt.reservationId : receipt.id}</span>
      </span>
      <span className="shrink-0 text-right">
        <span className={`block font-mono text-sm font-semibold ${positive ? 'text-emerald-500' : 'text-foreground'}`}>{positive ? '+' : ''}{amount} cr</span>
        {!isEvent && <span className="text-[11px] capitalize text-muted-foreground">{receipt.status}</span>}
      </span>
    </button>
  );
}

export function CreditsPage() {
  const { toast } = useToast();
  const [account, setAccount] = useState<CreditAccountResponse | null>(null);
  const [usage, setUsage] = useState<CreditUsageResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [selectedReceipt, setSelectedReceipt] = useState<CreditReceipt | null>(null);
  const [copied, setCopied] = useState(false);

  const load = async (quiet = false) => {
    quiet ? setRefreshing(true) : setLoading(true);
    try {
      const [nextAccount, nextUsage] = await Promise.all([creditApi.account(), creditApi.usage()]);
      setAccount(nextAccount);
      setUsage(nextUsage);
    } catch (error) {
      toast({ title: creditErrorMessage(error, 'Credit details are unavailable'), variant: 'destructive' });
    } finally {
      setLoading(false);
      setRefreshing(false);
    }
  };

  useEffect(() => { void load(); }, []);

  const receipts = useMemo(() => {
    if (!usage) return [];
    return [...(usage.recent.reservations ?? []), ...(usage.recent.events ?? [])]
      .sort((a, b) => new Date(b.createdAt ?? 0).getTime() - new Date(a.createdAt ?? 0).getTime())
      .slice(0, 25);
  }, [usage]);

  const copyReceipt = async () => {
    if (!selectedReceipt) return;
    await navigator.clipboard?.writeText(JSON.stringify(selectedReceipt, null, 2));
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1600);
  };

  if (loading) {
    return <PageTransition className="mx-auto max-w-5xl space-y-6"><Skeleton className="h-12 w-56" /><div className="grid gap-4 md:grid-cols-3"><Skeleton className="h-36" /><Skeleton className="h-36" /><Skeleton className="h-36" /></div><Skeleton className="h-96" /></PageTransition>;
  }

  const balance = account?.balance ?? 0;
  const usageItems = [
    { label: 'Granted', value: account?.granted ?? 0, tone: 'text-sky-400' },
    { label: 'Adjustments', value: account?.adjustments ?? 0, tone: 'text-violet-400' },
    { label: 'Reserved', value: account?.reserved ?? 0, tone: 'text-amber-400' },
    { label: 'Spent', value: account?.spent ?? 0, tone: 'text-rose-400' },
    { label: 'Refunded', value: account?.refunded ?? 0, tone: 'text-emerald-400' },
  ];

  return (
    <PageTransition className="mx-auto max-w-5xl space-y-6">
      <header className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end">
        <div>
          <p className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.22em] text-primary"><WalletCards className="h-3.5 w-3.5" /> Account credits</p>
          <h1 className="text-3xl font-display font-bold tracking-tight md:text-4xl">Your AI wallet</h1>
          <p className="mt-2 max-w-xl text-muted-foreground">A clear record of what Askolo reserved, spent, and returned. Voice sessions check this balance before they start.</p>
        </div>
        <Button variant="outline" onClick={() => void load(true)} disabled={refreshing} data-testid="button-refresh-credits">
          <RefreshCw className={`mr-2 h-4 w-4 ${refreshing ? 'animate-spin' : ''}`} /> Refresh
        </Button>
      </header>

      <section className="grid gap-4 md:grid-cols-[1.35fr_1fr_1fr]">
        <Card className="relative overflow-hidden border-primary/30 bg-primary/[0.07]">
          <div className="pointer-events-none absolute -right-10 -top-14 h-40 w-40 rounded-full bg-primary/10 blur-3xl" />
          <CardHeader className="pb-2"><CardDescription className="flex items-center gap-2"><Coins className="h-4 w-4 text-primary" /> Available now</CardDescription></CardHeader>
          <CardContent><p className="font-mono text-5xl font-semibold tracking-tight" data-testid="text-credit-balance">{balance.toLocaleString()}<span className="ml-2 text-base font-sans font-medium text-muted-foreground">credits</span></p><p className="mt-3 text-xs text-muted-foreground">Enforcement is strict. A voice request needs at least one available credit.</p></CardContent>
        </Card>
        <Card><CardHeader className="pb-2"><CardDescription>Policy version</CardDescription></CardHeader><CardContent><p className="font-mono text-3xl font-semibold">v{account?.policyVersion ?? '—'}</p><Badge className="mt-3 border-emerald-500/20 bg-emerald-500/10 text-emerald-500"><ShieldCheck className="mr-1 h-3 w-3" /> Strict ledger</Badge></CardContent></Card>
        <Card><CardHeader className="pb-2"><CardDescription>Voice preflight</CardDescription></CardHeader><CardContent><p className="text-sm font-medium">1 credit per request</p><p className={`mt-2 text-xs ${balance > 0 ? 'text-emerald-500' : 'text-destructive'}`}>{balance > 0 ? 'Ready for a voice note' : 'Top up or ask an admin for credits'}</p></CardContent></Card>
      </section>

      <Card>
        <CardHeader><CardTitle className="flex items-center gap-2 text-lg"><History className="h-4 w-4 text-primary" /> Ledger summary</CardTitle><CardDescription>Every number is an immutable ledger counter, not an estimate.</CardDescription></CardHeader>
        <CardContent className="grid grid-cols-2 gap-3 sm:grid-cols-5">{usageItems.map((item) => <div key={item.label} className="rounded-xl border border-border/70 bg-background/30 p-3"><p className="text-xs text-muted-foreground">{item.label}</p><p className={`mt-1 font-mono text-xl font-semibold ${item.tone}`}>{item.value.toLocaleString()}</p></div>)}</CardContent>
      </Card>

      <Card>
        <CardHeader className="flex flex-row items-start justify-between gap-3"><div><CardTitle className="flex items-center gap-2 text-lg"><Sparkles className="h-4 w-4 text-primary" /> Receipts</CardTitle><CardDescription>Recent reservations and settlement events.</CardDescription></div><Button variant="ghost" size="sm" onClick={() => void load(true)} data-testid="button-refresh-receipts"><RefreshCw className="mr-2 h-3.5 w-3.5" /> Sync</Button></CardHeader>
        <CardContent className="space-y-2">{receipts.length ? receipts.map((receipt, index) => <ReceiptRow key={`${receipt.id}-${index}`} receipt={receipt} onOpen={() => setSelectedReceipt(receipt)} />) : <div className="rounded-xl border border-dashed border-border p-10 text-center"><Download className="mx-auto mb-3 h-7 w-7 text-muted-foreground/50" /><p className="font-medium">No receipts yet</p><p className="mt-1 text-sm text-muted-foreground">Your first AI or voice operation will appear here.</p></div>}</CardContent>
      </Card>

      <Dialog open={Boolean(selectedReceipt)} onOpenChange={(open) => !open && setSelectedReceipt(null)}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader><DialogTitle>Credit receipt</DialogTitle><DialogDescription>Reference details for this ledger event.</DialogDescription></DialogHeader>
          {selectedReceipt && <div className="rounded-xl border border-border bg-background/50 p-4"><dl className="grid grid-cols-2 gap-x-4 gap-y-3 text-sm">{Object.entries(selectedReceipt).filter(([, value]) => value !== null && value !== undefined).map(([key, value]) => <div key={key}><dt className="text-xs capitalize text-muted-foreground">{key.replace(/[A-Z]/g, (letter) => ` ${letter}`).trim()}</dt><dd className="mt-0.5 break-all font-mono text-xs">{typeof value === 'object' ? JSON.stringify(value) : String(value)}</dd></div>)}</dl></div>}
          <DialogFooter><Button variant="outline" onClick={() => void copyReceipt()} data-testid="button-copy-receipt">{copied ? <Check className="mr-2 h-4 w-4" /> : <Clipboard className="mr-2 h-4 w-4" />}{copied ? 'Copied' : 'Copy receipt JSON'}</Button><Button onClick={() => setSelectedReceipt(null)}>Done</Button></DialogFooter>
        </DialogContent>
      </Dialog>
    </PageTransition>
  );
}
import { useEffect, useMemo, useState } from 'react';
import { Link, useLocation } from 'wouter';
import { ArrowDownLeft, ArrowUpRight, Check, Clipboard, History, RefreshCw, WalletCards } from 'lucide-react';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { PageTransition } from '@/components/ui/page-transition';
import { VoiceCreditPreflight } from '@/components/credits/voice-credit-preflight';
import { useToast } from '@/hooks/use-toast';
import { getConductedCreditActivities, type CreditActivity } from '@/lib/credit-activity';
import { creditApi, creditErrorMessage, formatCredits, formatUsdMicros, type CreditAccountResponse, type CreditUsageResponse } from '@/lib/credit-api';

function amount(item: CreditActivity) { return 'eventType' in item ? item.amountUsdMicros : item.settledUsdMicros; }
function ReceiptRow({ item, onOpen }: { item: CreditActivity; onOpen: () => void }) {
  const event = 'eventType' in item;
  const positive = event && (item.eventType.includes('refund') || item.eventType.includes('return'));
  return <button type="button" onClick={onOpen} data-testid={`button-open-receipt-${item.id}`} className="group flex w-full items-center gap-3 rounded-xl border border-border/70 bg-background/35 p-3 text-start hover:border-primary/40 hover:bg-primary/5">
    <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border ${positive ? 'border-success/30 bg-success/10 text-success' : 'border-primary/25 bg-primary/10 text-primary'}`}>{positive ? <ArrowDownLeft className="h-4 w-4" /> : <ArrowUpRight className="h-4 w-4" />}</span>
    <span className="min-w-0 flex-1"><span className="block truncate text-sm font-medium">{event ? item.eventType : `${item.operationType} operation`}</span><span className="block truncate text-xs text-muted-foreground">{new Date(item.createdAt).toLocaleString()} · <bdi dir="ltr">{event ? item.reservationId : item.id}</bdi></span></span>
    <span className={`shrink-0 font-mono text-sm font-semibold ${positive ? 'text-success' : ''}`}><bdi>{positive ? '+' : ''}{formatUsdMicros(amount(item))}</bdi></span>
  </button>;
}
export function CreditsPage() {
  const [location] = useLocation();
  const billingTab = location.endsWith('/billing');
  const { toast } = useToast();
  const [account, setAccount] = useState<CreditAccountResponse | null>(null);
  const [usage, setUsage] = useState<CreditUsageResponse | null>(null);
  const [loading, setLoading] = useState(true); const [refreshing, setRefreshing] = useState(false); const [error, setError] = useState('');
  const [selected, setSelected] = useState<CreditActivity | null>(null); const [copied, setCopied] = useState(false);
  const load = async (quiet = false) => { quiet ? setRefreshing(true) : setLoading(true); setError(''); try { const [a, u] = await Promise.all([creditApi.account(), creditApi.usage()]); setAccount(a); setUsage(u); } catch (e) { const message = creditErrorMessage(e, 'Credit usage details are unavailable'); setError(message); toast({ title: message, variant: 'destructive' }); } finally { setLoading(false); setRefreshing(false); } };
  useEffect(() => { void load(); }, []);
  const activities = useMemo(() => usage ? getConductedCreditActivities(usage).slice(0, 25) : [], [usage]);
  if (loading) return <PageTransition surface={false} className="mx-auto max-w-5xl space-y-6"><Skeleton className="h-12 w-56" /><div className="grid gap-4 md:grid-cols-3"><Skeleton className="h-36" /><Skeleton className="h-36" /><Skeleton className="h-36" /></div><Skeleton className="h-96" /></PageTransition>;
   if (!account || !usage) return <PageTransition surface={false} className="mx-auto max-w-3xl"><Card role="alert"><CardHeader><CardTitle>Credit details are unavailable</CardTitle><CardDescription>{error || 'Refresh to load your auditable wallet and activity.'}</CardDescription></CardHeader><CardContent><Button onClick={() => void load()}>Try again</Button></CardContent></Card></PageTransition>;
  const counters = [['Granted', account.grantedUsdMicros], ['Adjustments', account.adjustmentsUsdMicros], ['Reserved', account.reservedUsdMicros], ['Spent', account.spentUsdMicros], ['Refunded', account.refundedUsdMicros]];
  return <PageTransition surface={false} className="mx-auto max-w-5xl space-y-6">
    {error && <p role="status" className="rounded-lg border border-warning/30 bg-warning/10 p-3 text-sm text-warning">{error} The last loaded balance is shown.</p>}
       <header className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end"><div><p className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.22em] text-primary"><WalletCards className="h-3.5 w-3.5" /> Personal credits</p><h1 className="text-3xl font-display font-bold tracking-tight md:text-4xl">{billingTab ? 'Credits' : 'Usage'}</h1><p className="mt-2 max-w-xl text-muted-foreground">{billingTab ? 'Your available credits, ledger, and receipts.' : 'Review completed assistant and voice operations charged to your personal wallet.'}</p></div><div className="flex flex-wrap gap-2">{account.canManage && <Link href="/admin/credits" className="inline-flex min-h-10 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground">Manage policy</Link>}<Button variant="outline" onClick={() => void load(true)} disabled={refreshing}><RefreshCw className="me-2 h-4 w-4" /> Refresh</Button></div></header>
      <nav aria-label="Personal account settings" className="flex gap-2 border-b border-border">
        <Link href="/settings/account/profile" className="min-h-11 border-b-2 border-transparent px-3 py-2 text-sm font-medium text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">Account</Link>
        <Link href="/settings/space/details" className="min-h-11 border-b-2 border-transparent px-3 py-2 text-sm font-medium text-muted-foreground hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">Space details</Link>
      </nav>
       <nav aria-label="Personal credit settings" className="flex gap-2 border-b border-border">
        <Link href="/settings/space/usage" aria-current={!billingTab ? 'page' : undefined} className={`min-h-11 border-b-2 px-3 py-2 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${!billingTab ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'}`}>Usage</Link>
         <Link href="/settings/space/billing" aria-current={billingTab ? 'page' : undefined} className={`min-h-11 border-b-2 px-3 py-2 text-sm font-medium focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring ${billingTab ? 'border-primary text-foreground' : 'border-transparent text-muted-foreground hover:text-foreground'}`}>Credits</Link>
      </nav>
       {billingTab && <section><Card className="border-primary/30 bg-primary/[0.07]"><CardHeader className="pb-2"><CardDescription>Available now</CardDescription></CardHeader><CardContent><p className="font-mono text-5xl font-semibold tracking-tight" data-testid="text-credit-balance">{formatCredits(account.balanceUsdMicros)}</p><p className="mt-3 text-xs text-muted-foreground">The estimate is checked again when an operation starts.</p></CardContent></Card></section>}
      {!billingTab && <VoiceCreditPreflight showWalletLink={false} showPolicyVersion={false} />}
     {billingTab && <section aria-labelledby="ledger-counters-title" className="space-y-4">
      <div>
        <h2 id="ledger-counters-title" className="flex items-center gap-2 text-lg font-semibold"><History className="h-4 w-4 text-primary" /> Ledger counters</h2>
         <p className="mt-1 text-sm text-muted-foreground">Credit totals across your account ledger.</p>
      </div>
       <div className="grid grid-cols-2 gap-3 sm:grid-cols-5">{counters.map(([label, value]) => <div key={label} className="rounded-xl border border-border/70 bg-background/30 p-3"><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 font-mono text-lg font-semibold">{formatCredits(value as number)}</p></div>)}</div>
     </section>}
      <Card><CardHeader><CardTitle>{billingTab ? 'Ledger and receipts' : 'Usage activity'}</CardTitle><CardDescription>{billingTab ? 'Completed operations and refunds are shown here. Temporary holds and releases remain in the auditable ledger but are hidden from activity.' : 'Completed operations and refunds charged to your personal wallet.'}</CardDescription></CardHeader><CardContent className="space-y-2">{activities.length ? activities.map((item) => <ReceiptRow key={`${'eventType' in item ? 'event' : 'reservation'}-${item.id}`} item={item} onOpen={() => setSelected(item)} />) : <div className="rounded-xl border border-dashed border-border p-10 text-center"><p className="font-medium">No completed usage yet</p><p className="mt-1 text-sm text-muted-foreground">Your first completed assistant or voice operation will appear here.</p></div>}</CardContent></Card>
    <Dialog open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(null)}><DialogContent><DialogHeader><DialogTitle>USD receipt details</DialogTitle><DialogDescription>Reference details for this ledger event.</DialogDescription></DialogHeader>{selected && <pre className="max-h-80 overflow-auto rounded-xl border border-border bg-background/50 p-4 text-xs" dir="ltr">{JSON.stringify(selected, null, 2)}</pre>}<DialogFooter><Button variant="outline" onClick={() => { if (selected) void navigator.clipboard?.writeText(JSON.stringify(selected, null, 2)); setCopied(true); window.setTimeout(() => setCopied(false), 1200); }}><Clipboard className="me-2 h-4 w-4" />{copied ? 'Copied' : 'Copy receipt JSON'}</Button><Button onClick={() => setSelected(null)}>{copied ? <Check className="me-2 h-4 w-4" /> : null}Done</Button></DialogFooter></DialogContent></Dialog>
  </PageTransition>;
}
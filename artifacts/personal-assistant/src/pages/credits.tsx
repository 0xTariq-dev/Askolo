import { useEffect, useMemo, useState } from 'react';
import { Link } from 'wouter';
import { ArrowDownLeft, ArrowUpRight, Check, Clipboard, History, RefreshCw, ShieldCheck, WalletCards } from 'lucide-react';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card';
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog';
import { Skeleton } from '@/components/ui/skeleton';
import { PageTransition } from '@/components/ui/page-transition';
import { VoiceCreditPreflight } from '@/components/credits/voice-credit-preflight';
import { useToast } from '@/hooks/use-toast';
import { creditApi, creditErrorMessage, formatUsdMicros, type CreditAccountResponse, type CreditEvent, type CreditReservation, type CreditUsageResponse } from '@/lib/credit-api';

type Activity = CreditEvent | CreditReservation;
function amount(item: Activity) { return 'eventType' in item ? item.amountUsdMicros : item.settledUsdMicros || item.reservedUsdMicros; }
function ReceiptRow({ item, onOpen }: { item: Activity; onOpen: () => void }) {
  const event = 'eventType' in item;
  const positive = event && (item.eventType.includes('refund') || item.eventType.includes('return'));
  return <button type="button" onClick={onOpen} data-testid={`button-open-receipt-${item.id}`} className="group flex w-full items-center gap-3 rounded-xl border border-border/70 bg-background/35 p-3 text-start hover:border-primary/40 hover:bg-primary/5">
    <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-lg border ${positive ? 'border-success/30 bg-success/10 text-success' : 'border-primary/25 bg-primary/10 text-primary'}`}>{positive ? <ArrowDownLeft className="h-4 w-4" /> : <ArrowUpRight className="h-4 w-4" />}</span>
    <span className="min-w-0 flex-1"><span className="block truncate text-sm font-medium">{event ? item.eventType : `${item.operationType} operation`}</span><span className="block truncate text-xs text-muted-foreground">{new Date(item.createdAt).toLocaleString()} · <bdi dir="ltr">{event ? item.reservationId : item.id}</bdi></span></span>
    <span className={`shrink-0 font-mono text-sm font-semibold ${positive ? 'text-success' : ''}`}><bdi>{positive ? '+' : ''}{formatUsdMicros(amount(item))}</bdi></span>
  </button>;
}
export function CreditsPage() {
  const { toast } = useToast();
  const [account, setAccount] = useState<CreditAccountResponse | null>(null);
  const [usage, setUsage] = useState<CreditUsageResponse | null>(null);
  const [loading, setLoading] = useState(true); const [refreshing, setRefreshing] = useState(false); const [error, setError] = useState('');
  const [selected, setSelected] = useState<Activity | null>(null); const [copied, setCopied] = useState(false);
  const load = async (quiet = false) => { quiet ? setRefreshing(true) : setLoading(true); setError(''); try { const [a, u] = await Promise.all([creditApi.account(), creditApi.usage()]); setAccount(a); setUsage(u); } catch (e) { const message = creditErrorMessage(e, 'USD usage details are unavailable'); setError(message); toast({ title: message, variant: 'destructive' }); } finally { setLoading(false); setRefreshing(false); } };
  useEffect(() => { void load(); }, []);
  const activities = useMemo(() => usage ? [...usage.reservations, ...usage.events].sort((a, b) => new Date(b.createdAt).getTime() - new Date(a.createdAt).getTime()).slice(0, 25) : [], [usage]);
  if (loading) return <PageTransition className="mx-auto max-w-5xl space-y-6"><Skeleton className="h-12 w-56" /><div className="grid gap-4 md:grid-cols-3"><Skeleton className="h-36" /><Skeleton className="h-36" /><Skeleton className="h-36" /></div><Skeleton className="h-96" /></PageTransition>;
  if (!account || !usage) return <PageTransition className="mx-auto max-w-3xl"><Card role="alert"><CardHeader><CardTitle>USD usage details are unavailable</CardTitle><CardDescription>{error || 'Refresh to load your auditable wallet and activity.'}</CardDescription></CardHeader><CardContent><Button onClick={() => void load()}>Try again</Button></CardContent></Card></PageTransition>;
  const counters = [['Granted', account.grantedUsdMicros], ['Adjustments', account.adjustmentsUsdMicros], ['Reserved', account.reservedUsdMicros], ['Spent', account.spentUsdMicros], ['Refunded', account.refundedUsdMicros]];
  return <PageTransition className="mx-auto max-w-5xl space-y-6">
    {error && <p role="status" className="rounded-lg border border-warning/30 bg-warning/10 p-3 text-sm text-warning">{error} The last loaded balance is shown.</p>}
    <header className="flex flex-col justify-between gap-4 sm:flex-row sm:items-end"><div><p className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.22em] text-primary"><WalletCards className="h-3.5 w-3.5" /> Personal USD wallet</p><h1 className="text-3xl font-display font-bold tracking-tight md:text-4xl">AI usage ledger</h1><p className="mt-2 max-w-xl text-muted-foreground">Understand exactly what Askolo reserved, settled, and returned. Amounts are shown in US dollars, not abstract credits.</p></div><div className="flex flex-wrap gap-2">{account.canManage && <Link href="/admin/credits" className="inline-flex min-h-10 items-center rounded-md bg-primary px-4 text-sm font-medium text-primary-foreground">Manage policy</Link>}<Button variant="outline" onClick={() => void load(true)} disabled={refreshing}><RefreshCw className="me-2 h-4 w-4" /> Refresh</Button></div></header>
    <section className="grid gap-4 md:grid-cols-[1.35fr_1fr]"><Card className="border-primary/30 bg-primary/[0.07]"><CardHeader className="pb-2"><CardDescription>Available now</CardDescription></CardHeader><CardContent><p className="font-mono text-5xl font-semibold tracking-tight" data-testid="text-usd-balance">{formatUsdMicros(account.balanceUsdMicros)}</p><p className="mt-3 text-xs text-muted-foreground">Strict enforcement. The estimate is checked again when an operation starts.</p></CardContent></Card><Card><CardHeader className="pb-2"><CardDescription>Policy version</CardDescription></CardHeader><CardContent><p className="font-mono text-3xl font-semibold">v{account.policyVersion}</p><Badge className="mt-3 border-success/20 bg-success/10 text-success"><ShieldCheck className="me-1 h-3 w-3" /> Auditable ledger</Badge></CardContent></Card></section>
    <VoiceCreditPreflight showWalletLink={false} />
    <Card><CardHeader><CardTitle className="flex items-center gap-2 text-lg"><History className="h-4 w-4 text-primary" /> Ledger counters</CardTitle><CardDescription>Fixed-point USD microunits. 1 USD equals 1,000,000 microunits.</CardDescription></CardHeader><CardContent className="grid grid-cols-2 gap-3 sm:grid-cols-5">{counters.map(([label, value]) => <div key={label} className="rounded-xl border border-border/70 bg-background/30 p-3"><p className="text-xs text-muted-foreground">{label}</p><p className="mt-1 font-mono text-lg font-semibold">{formatUsdMicros(value as number)}</p></div>)}</CardContent></Card>
    <Card><CardHeader><CardTitle>Activity and receipts</CardTitle><CardDescription>Reservations and settlement events remain available for review.</CardDescription></CardHeader><CardContent className="space-y-2">{activities.length ? activities.map((item) => <ReceiptRow key={`${'eventType' in item ? 'event' : 'reservation'}-${item.id}`} item={item} onOpen={() => setSelected(item)} />) : <div className="rounded-xl border border-dashed border-border p-10 text-center"><p className="font-medium">No usage activity yet</p><p className="mt-1 text-sm text-muted-foreground">Your first assistant or voice operation will appear here.</p></div>}</CardContent></Card>
    <Dialog open={Boolean(selected)} onOpenChange={(open) => !open && setSelected(null)}><DialogContent><DialogHeader><DialogTitle>USD receipt details</DialogTitle><DialogDescription>Reference details for this ledger event.</DialogDescription></DialogHeader>{selected && <pre className="max-h-80 overflow-auto rounded-xl border border-border bg-background/50 p-4 text-xs" dir="ltr">{JSON.stringify(selected, null, 2)}</pre>}<DialogFooter><Button variant="outline" onClick={() => { if (selected) void navigator.clipboard?.writeText(JSON.stringify(selected, null, 2)); setCopied(true); window.setTimeout(() => setCopied(false), 1200); }}><Clipboard className="me-2 h-4 w-4" />{copied ? 'Copied' : 'Copy receipt JSON'}</Button><Button onClick={() => setSelected(null)}>{copied ? <Check className="me-2 h-4 w-4" /> : null}Done</Button></DialogFooter></DialogContent></Dialog>
  </PageTransition>;
}
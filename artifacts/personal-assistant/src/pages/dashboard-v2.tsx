import { PageTransition } from '@/components/ui/page-transition';
import { HomeV2Summary } from '@/components/dashboard/home-v2-summary';
import { useLocale } from '@/contexts/locale-context';
import { HomeAgentComposer } from '@/components/dashboard/home-agent-composer';

export function DashboardPage() {
  const { locale } = useLocale();
  const isArabic = locale === 'ar';

  return (
    <PageTransition surface={false} className="mx-auto flex w-full max-w-7xl flex-col gap-6 pb-10">
      <HomeAgentComposer />
      <header className="border-b border-border pb-6">
        <h1 className="mt-2 text-3xl font-bold tracking-tight md:text-4xl">
          {isArabic ? 'نظرة على يومك' : 'Your day, at a glance'}
        </h1>
        <p className="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground md:text-base">
          {isArabic
            ? 'ملخص موجز للتركيز والبريد الإلكتروني والتقويم لهذا اليوم.'
            : 'A concise view of today’s focus, email, and calendar.'}
        </p>
      </header>
      <HomeV2Summary />
    </PageTransition>
  );
}

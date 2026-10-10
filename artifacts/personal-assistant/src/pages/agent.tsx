import { Sparkles } from 'lucide-react';
import { PageTransition } from '@/components/ui/page-transition';
import { AssistantSidebar } from '@/components/assistant-sidebar';
import { useLocale } from '@/contexts/locale-context';
import { useSpaceScope } from '@/contexts/space-context';

export function AgentPage() {
  const { locale } = useLocale();
  const { spaces, selectedSpaceId } = useSpaceScope();
  const isArabic = locale === 'ar';
  const selectedSpace = spaces.find((space) => space.id === selectedSpaceId);

  return (
    <PageTransition surface={false} className="mx-auto flex w-full max-w-6xl flex-col gap-5">
      <header className="flex flex-col justify-between gap-3 sm:flex-row sm:items-end">
        <div>
          <p className="mb-2 flex items-center gap-2 text-xs font-semibold uppercase tracking-[0.18em] text-primary">
            <Sparkles className="size-4" aria-hidden="true" />
            {isArabic ? 'مساحة أسكولو' : 'ASKOLO WORKSPACE'}
          </p>
          <h1 className="text-3xl font-bold tracking-tight md:text-4xl">
            {isArabic ? 'مساعدك الشخصي' : 'Your personal assistant'}
          </h1>
          <p className="mt-2 max-w-2xl text-sm leading-relaxed text-muted-foreground md:text-base">
            {isArabic
              ? 'تحدث عن يومك، رتّب أفكارك، وراجع كل إجراء قبل تنفيذه.'
              : 'Think through your day, shape your next step, and review every action before it happens.'}
          </p>
          <p className="mt-2 text-sm font-medium text-primary" role="status">
            {selectedSpace
              ? `${isArabic ? 'المساحة النشطة' : 'Active Space'}: ${selectedSpace.name}`
              : (isArabic ? 'نطاق كل المساحات غير متاح للوكيل بعد.' : 'Agent does not support All Spaces scope yet.')}
          </p>
        </div>
      </header>
      <AssistantSidebar fullPage />
    </PageTransition>
  );
}

import { PageTransition } from '@/components/ui/page-transition';
import { HomeV2Summary } from '@/components/dashboard/home-v2-summary';
import { HomeAgentComposer } from '@/components/dashboard/home-agent-composer';
import { HomeGreeting } from '@/components/dashboard/home-greeting';

export function DashboardPage() {
  return (
    <PageTransition surface={false} className="mx-auto flex w-full max-w-7xl flex-col gap-6 pb-10">
      <HomeGreeting />
      <HomeAgentComposer />
      <HomeV2Summary />
    </PageTransition>
  );
}

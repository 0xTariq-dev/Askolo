import { PageTransition } from '@/components/ui/page-transition';
import { AssistantSidebar } from '@/components/assistant-sidebar';

export function AgentPage() {
  return (
    <PageTransition surface={false} className="mx-auto flex min-h-[calc(100dvh-9rem)] w-full max-w-6xl flex-col">
      <AssistantSidebar fullPage />
    </PageTransition>
  );
}

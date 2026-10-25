import type { ReactNode } from 'react';
import { SpaceSettingsNavigation } from '@/components/settings/space-settings-navigation';
import { PageTransition } from '@/components/ui/page-transition';

export function SettingsLayout({ children }: { children: ReactNode }) {
  return (
    <PageTransition surface={false} className="settings-area mx-auto w-full max-w-7xl pb-10">
      <div className="grid min-w-0 gap-5 md:grid-cols-[13rem_minmax(0,1fr)] lg:grid-cols-[15rem_minmax(0,1fr)] lg:gap-7">
        <aside className="min-w-0 md:sticky md:top-5 md:self-start">
          <SpaceSettingsNavigation />
        </aside>
        <main className="min-w-0 space-y-6">{children}</main>
      </div>
    </PageTransition>
  );
}

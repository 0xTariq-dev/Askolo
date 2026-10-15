import { lazy } from 'react';

export const AppLayout = lazy(() =>
  import('@/components/layout/app-layout').then(({ AppLayout }) => ({ default: AppLayout })),
);

export const SidebarAppLayout = lazy(() =>
  import('@/components/layout/app-sidebar-layout').then(({ SidebarAppLayout }) => ({
    default: SidebarAppLayout,
  })),
);

export const DashboardPage = lazy(() =>
  import('@/pages/dashboard-v2').then(({ DashboardPage }) => ({ default: DashboardPage })),
);

export const GoalsPage = lazy(() =>
  import('@/pages/goals').then(({ GoalsPage }) => ({ default: GoalsPage })),
);

export const PlanPage = lazy(() =>
  import('@/pages/plan').then(({ PlanPage }) => ({ default: PlanPage })),
);

export const AgentPage = lazy(() =>
  import('@/pages/agent').then(({ AgentPage }) => ({ default: AgentPage })),
);

export const CalendarPage = lazy(() =>
  import('@/pages/calendar').then(({ CalendarPage }) => ({ default: CalendarPage })),
);

export const NotesPage = lazy(() =>
  import('@/pages/notes').then(({ NotesPage }) => ({ default: NotesPage })),
);

export const EmailPage = lazy(() =>
  import('@/pages/email').then(({ EmailPage }) => ({ default: EmailPage })),
);

export const ProfilePage = lazy(() =>
  import('@/pages/profile').then(({ ProfilePage }) => ({ default: ProfilePage })),
);

export const SpaceDetailsPage = lazy(() =>
  import('@/pages/space-details').then(({ SpaceDetailsPage }) => ({ default: SpaceDetailsPage })),
);
export const SpaceSlugRoute = lazy(() =>
  import('@/pages/space-slug-route').then(({ SpaceSlugRoute }) => ({ default: SpaceSlugRoute })),
);
export const SpaceNotificationsPage = lazy(() =>
  import('@/pages/space-notifications').then(({ SpaceNotificationsPage }) => ({ default: SpaceNotificationsPage })),
);
export const SpaceMembersPage = lazy(() =>
  import('@/pages/space-members').then(({ SpaceMembersPage }) => ({ default: SpaceMembersPage })),
);

export const InviteAcceptancePage = lazy(() =>
  import('@/pages/invite-acceptance').then(({ InviteAcceptancePage }) => ({ default: InviteAcceptancePage })),
);

export const CreditsPage = lazy(() =>
  import('@/pages/credits').then(({ CreditsPage }) => ({ default: CreditsPage })),
);

export const AdminCreditsPage = lazy(() =>
  import('@/pages/admin-credits').then(({ AdminCreditsPage }) => ({ default: AdminCreditsPage })),
);

export const LoginPage = lazy(() =>
  import('@/pages/login').then(({ LoginPage }) => ({ default: LoginPage })),
);

export const LandingPage = lazy(() =>
  import('@/pages/landing').then(({ LandingPage }) => ({ default: LandingPage })),
);

export const PrivacyPage = lazy(() =>
  import('@/pages/privacy').then(({ PrivacyPage }) => ({ default: PrivacyPage })),
);

export const TermsPage = lazy(() =>
  import('@/pages/terms').then(({ TermsPage }) => ({ default: TermsPage })),
);
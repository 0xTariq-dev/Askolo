import { lazy } from 'react';

export const AppLayout = lazy(() =>
  import('@/components/layout/app-layout').then(({ AppLayout }) => ({ default: AppLayout })),
);

export const DashboardPage = lazy(() =>
  import('@/pages/dashboard').then(({ DashboardPage }) => ({ default: DashboardPage })),
);

export const HabitsPage = lazy(() =>
  import('@/pages/habits').then(({ HabitsPage }) => ({ default: HabitsPage })),
);

export const GoalsPage = lazy(() =>
  import('@/pages/goals').then(({ GoalsPage }) => ({ default: GoalsPage })),
);

export const PlanPage = lazy(() =>
  import('@/pages/plan').then(({ PlanPage }) => ({ default: PlanPage })),
);

export const CalendarPage = lazy(() =>
  import('@/pages/calendar').then(({ CalendarPage }) => ({ default: CalendarPage })),
);

export const ChoresPage = lazy(() =>
  import('@/pages/chores').then(({ ChoresPage }) => ({ default: ChoresPage })),
);

export const NotesPage = lazy(() =>
  import('@/pages/notes').then(({ NotesPage }) => ({ default: NotesPage })),
);

export const ActionsPage = lazy(() =>
  import('@/pages/actions').then(({ ActionsPage }) => ({ default: ActionsPage })),
);

export const EmailPage = lazy(() =>
  import('@/pages/email').then(({ EmailPage }) => ({ default: EmailPage })),
);

export const ProfilePage = lazy(() =>
  import('@/pages/profile').then(({ ProfilePage }) => ({ default: ProfilePage })),
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
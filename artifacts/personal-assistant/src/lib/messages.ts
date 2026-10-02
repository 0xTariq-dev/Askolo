import { msg } from '@lingui/core/macro';

export const messages = {
  'common.language': msg({ id: 'common.language', message: 'Language' }),
  'common.english': msg({ id: 'common.english', message: 'English' }),
  'common.arabic': msg({ id: 'common.arabic', message: 'Arabic' }),
  'common.signIn': msg({ id: 'common.signIn', message: 'Sign In' }),
  'common.getStarted': msg({ id: 'common.getStarted', message: 'Get Started' }),
  'common.features': msg({ id: 'common.features', message: 'Features' }),
  'common.integrations': msg({ id: 'common.integrations', message: 'Integrations' }),
  'common.privacyPolicy': msg({ id: 'common.privacyPolicy', message: 'Privacy Policy' }),
  'common.termsOfService': msg({ id: 'common.termsOfService', message: 'Terms of Service' }),
  'common.switchToLight': msg({
    id: 'common.switchToLight',
    message: 'Switch to light appearance',
  }),
  'common.switchToDark': msg({
    id: 'common.switchToDark',
    message: 'Switch to dark appearance',
  }),
  'nav.dashboard': msg({ id: 'nav.dashboard', message: 'Dashboard' }),
  'nav.habits': msg({ id: 'nav.habits', message: 'Habits' }),
  'nav.goals': msg({ id: 'nav.goals', message: 'Goals' }),
  'nav.dailyPlan': msg({ id: 'nav.dailyPlan', message: 'Daily Plan' }),
  'nav.calendar': msg({ id: 'nav.calendar', message: 'Calendar' }),
  'nav.chores': msg({ id: 'nav.chores', message: 'Chores' }),
  'nav.notes': msg({ id: 'nav.notes', message: 'Notes' }),
  'nav.actions': msg({ id: 'nav.actions', message: 'Actions' }),
  'nav.email': msg({ id: 'nav.email', message: 'Email' }),
  'nav.aiCredits': msg({ id: 'nav.aiCredits', message: 'Balance' }),
  'nav.signOut': msg({ id: 'nav.signOut', message: 'Sign Out' }),
  'nav.main': msg({ id: 'nav.main', message: 'Main navigation' }),
  'nav.openMenu': msg({ id: 'nav.openMenu', message: 'Open navigation menu' }),
  'nav.skipToMain': msg({ id: 'nav.skipToMain', message: 'Skip to main content' }),
  'nav.mobileTitle': msg({ id: 'nav.mobileTitle', message: 'Askolo navigation' }),
  'nav.mobileDescription': msg({
    id: 'nav.mobileDescription',
    message: 'Navigate between your Askolo workspaces.',
  }),
  'app.workspace': msg({ id: 'app.workspace', message: 'Askolo workspace' }),
  'app.overview': msg({ id: 'app.overview', message: 'Overview' }),
  'profile.language': msg({ id: 'profile.language', message: 'Language' }),
  'profile.languageDescription': msg({
    id: 'profile.languageDescription',
    message: 'Choose the language and reading direction used in Askolo.',
  }),
  'profile.languageSyncFailed': msg({
    id: 'profile.languageSyncFailed',
    message: 'Could not sync your language preference. It is still applied on this device.',
  }),
  'profile.account': msg({ id: 'profile.account', message: 'Account' }),
  'calendar.allDay': msg({ id: 'calendar.allDay', message: 'All day' }),
  'calendar.today': msg({ id: 'calendar.today', message: 'Today' }),
  'calendar.previous': msg({ id: 'calendar.previous', message: 'Previous period' }),
  'calendar.next': msg({ id: 'calendar.next', message: 'Next period' }),
  'calendar.viewMonth': msg({ id: 'calendar.viewMonth', message: 'Month' }),
  'calendar.viewWeek': msg({ id: 'calendar.viewWeek', message: 'Week' }),
  'calendar.viewDay': msg({ id: 'calendar.viewDay', message: 'Day' }),
  'calendar.lastSynced': msg({ id: 'calendar.lastSynced', message: 'Last synced {time}' }),
  'calendar.notSynced': msg({ id: 'calendar.notSynced', message: 'Not yet synced this session' }),
  'calendar.moreEvents': msg({
    id: 'calendar.moreEvents',
    message:
      '{count, plural, one {{formattedCount} more event} other {{formattedCount} more events}}',
  }),
} as const;

export type MessageKey = keyof typeof messages;
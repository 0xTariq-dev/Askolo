import { useMemo, type ReactNode } from 'react';
import { useAuth, useClerk, useUser } from '@clerk/react';

import { AppAuthContext, type AppUser } from '@/contexts/auth-context';

export function ClerkAuthBridge({ children }: { children: ReactNode }) {
  const { isLoaded, isSignedIn } = useAuth();
  const { user } = useUser();
  const clerk = useClerk();

  const value = useMemo(
    () => ({
      isLoaded,
      isSignedIn: Boolean(isSignedIn && user),
      authProvider: isSignedIn ? ('clerk' as const) : null,
      user: user
        ? ({
            id: user.id,
            email: user.primaryEmailAddress?.emailAddress ?? null,
            firstName: user.firstName,
            lastName: user.lastName,
            profileImageUrl: user.imageUrl,
            imageUrl: user.imageUrl,
          } satisfies AppUser)
        : null,
      signOut: async (options?: { redirectUrl?: string }) => {
        await clerk.signOut(options);
      },
      updateProfile: async ({ firstName, lastName }: { firstName: string; lastName: string }) => {
        await user?.update({ firstName, lastName });
      },
      updateProfileImage: async (file: File) => {
        await user?.setProfileImage({ file });
      },
      updatePassword: async (currentPassword: string, newPassword: string) => {
        await user?.updatePassword({ currentPassword, newPassword });
      },
    }),
    [clerk, isLoaded, isSignedIn, user],
  );

  return <AppAuthContext.Provider value={value}>{children}</AppAuthContext.Provider>;
}
import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';

import { ApiError } from '@workspace/api-client-react';
import { toPublicUrl } from '@/lib/site-domains';
import { goApi } from '@/lib/go-api';
import type { Locale } from '@/lib/locale';

const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');

export type AppUser = {
  id: string;
  email: string | null;
  firstName: string | null;
  lastName: string | null;
  profileImageUrl: string | null;
  preferredLocale: Locale | null;
  imageUrl?: string;
  authProvider?: string;
  status?: string;
};

type SignOutOptions = { redirectUrl?: string };

type AppAuthValue = {
  user: AppUser | null;
  isLoaded: boolean;
  isSignedIn: boolean;
  mfaRequired: boolean;
  authProvider: string | null;
  signOut: (options?: SignOutOptions) => Promise<void>;
  updateProfile: (profile: {
    firstName?: string;
    lastName?: string;
    preferredLocale?: Locale;
  }) => Promise<void>;
  updateProfileImage: (file: File) => Promise<void>;
  updatePassword: (currentPassword: string, newPassword: string) => Promise<void>;
};

export const AppAuthContext = createContext<AppAuthValue | null>(null);

export function useAppAuth() {
  const value = useContext(AppAuthContext);
  if (!value) throw new Error('useAppAuth must be used within an auth provider');
  return value;
}

export function NativeAuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AppUser | null>(null);
  const [mfaRequired, setMfaRequired] = useState(false);
  const [isLoaded, setIsLoaded] = useState(false);

  useEffect(() => {
    let active = true;
    const controller = new AbortController();
    goApi.currentUser(controller.signal)
      .then((payload) => {
        if (!active) return;
        setUser(payload.user);
        setMfaRequired(payload.mfaRequired === true);
        setIsLoaded(true);
      })
      .catch(() => {
        if (!active) return;
        setUser(null);
        setMfaRequired(false);
        setIsLoaded(true);
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, []);

  const value = useMemo<AppAuthValue>(
    () => ({
      user,
      isLoaded,
      isSignedIn: Boolean(user),
      mfaRequired,
      authProvider: user?.authProvider ?? null,
      signOut: async (options) => {
        try {
          await goApi.logout();
        } catch (error) {
          console.warn('Logout request failed', {
            status: error instanceof ApiError ? error.status : undefined,
            code: error instanceof ApiError ? error.code : undefined,
            requestId: error instanceof ApiError ? error.requestId : undefined,
          });
        }
        window.location.assign(options?.redirectUrl ?? toPublicUrl('/'));
      },
      updateProfile: async (profile) => {
        const updated = await goApi.updateProfile(profile);
        setUser(updated);
      },
      updateProfileImage: async () => {
        throw new Error('Native profile photo editing is not available');
      },
      updatePassword: async (currentPassword, newPassword) => {
        await goApi.setPassword({ currentPassword, password: newPassword });
      },
    }),
    [isLoaded, mfaRequired, user],
  );

  return <AppAuthContext.Provider value={value}>{children}</AppAuthContext.Provider>;
}
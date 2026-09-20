import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';

import { toPublicUrl } from '@/lib/site-domains';

const basePath = import.meta.env.BASE_URL.replace(/\/$/, '');

export type AppUser = {
  id: string;
  email: string | null;
  firstName: string | null;
  lastName: string | null;
  profileImageUrl: string | null;
  imageUrl?: string;
  authProvider?: 'google' | 'github' | 'password';
  status?: string;
};

type SignOutOptions = { redirectUrl?: string };

type AppAuthValue = {
  user: AppUser | null;
  isLoaded: boolean;
  isSignedIn: boolean;
  authProvider: 'google' | 'github' | 'password' | null;
  signOut: (options?: SignOutOptions) => Promise<void>;
  updateProfile: (profile: { firstName: string; lastName: string }) => Promise<void>;
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
  const [isLoaded, setIsLoaded] = useState(false);

  useEffect(() => {
    let active = true;
    fetch(`${basePath}/api/auth/user`, { credentials: 'include' })
      .then(async (response) => {
        if (!response.ok) throw new Error(`Auth check failed: ${response.status}`);
        return (await response.json()) as { user: AppUser | null };
      })
      .then((payload) => {
        if (!active) return;
        setUser(payload.user);
        setIsLoaded(true);
      })
      .catch(() => {
        if (!active) return;
        setUser(null);
        setIsLoaded(true);
      });
    return () => {
      active = false;
    };
  }, []);

  const value = useMemo<AppAuthValue>(
    () => ({
      user,
      isLoaded,
      isSignedIn: Boolean(user),
      authProvider: user?.authProvider ?? null,
      signOut: async (options) => {
        await fetch(`${basePath}/api/auth/logout`, {
          method: 'POST',
          credentials: 'include',
        });
        window.location.assign(options?.redirectUrl ?? toPublicUrl('/'));
      },
      updateProfile: async ({ firstName, lastName }) => {
        const response = await fetch(`${basePath}/api/user/profile`, {
          method: 'PATCH',
          credentials: 'include',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ firstName, lastName }),
        });
        if (!response.ok) throw new Error('Profile update failed');
        const updated = (await response.json()) as AppUser;
        setUser(updated);
      },
      updateProfileImage: async () => {
        throw new Error('Native profile photo editing is not available');
      },
      updatePassword: async (currentPassword, newPassword) => {
        const response = await fetch(`${basePath}/api/auth/password/set`, {
          method: 'POST',
          credentials: 'include',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ currentPassword, password: newPassword }),
        });
        if (!response.ok) {
          const payload = (await response.json().catch(() => null)) as { error?: string } | null;
          throw new Error(payload?.error || 'Password update failed');
        }
      },
    }),
    [isLoaded, user],
  );

  return <AppAuthContext.Provider value={value}>{children}</AppAuthContext.Provider>;
}
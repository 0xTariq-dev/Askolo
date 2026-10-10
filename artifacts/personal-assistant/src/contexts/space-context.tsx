import { createContext, useContext, useEffect, useMemo, useState, type ReactNode } from 'react';
import { useAppAuth } from '@/contexts/auth-context';
import { getV2Workspaces, type V2Workspace } from '@/lib/agenda-v2-api';

const storageKeyForUser = (userId: string) => `askolo.selected-space.v2:${userId}`;
type SpaceContextValue = {
  spaces: V2Workspace[];
  selectedSpaceId: string | null;
  setSelectedSpaceId: (id: string | null) => void;
  loading: boolean;
  error: string | null;
};

const SpaceContext = createContext<SpaceContextValue | null>(null);

export function SpaceProvider({ children }: { children: ReactNode }) {
  const { isSignedIn, user } = useAppAuth();
  const [spaces, setSpaces] = useState<V2Workspace[]>([]);
  const [selectedSpaceId, setSelectedSpaceIdState] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (!isSignedIn || !user?.id) {
      setSpaces([]);
      setSelectedSpaceIdState(null);
      setLoading(false);
      setError(null);
      return;
    }
    const controller = new AbortController();
    const storageKey = storageKeyForUser(user.id);
    setSpaces([]);
    setSelectedSpaceIdState(null);
    setLoading(true);
    setError(null);
    getV2Workspaces(controller.signal).then((result) => {
      if (controller.signal.aborted) return;
      setSpaces(result);
      const saved = localStorage.getItem(storageKey);
      const initial = result.some((space) => space.id === saved)
        ? saved
        : result.find((space) => space.isPersonal)?.id ?? result[0]?.id ?? null;
      setSelectedSpaceIdState(initial);
      if (initial) localStorage.setItem(storageKey, initial);
      else localStorage.removeItem(storageKey);
    }).catch((loadError: unknown) => {
      if (controller.signal.aborted) return;
      setSpaces([]);
      setSelectedSpaceIdState(null);
      setError(loadError instanceof Error ? loadError.message : 'Spaces could not be loaded.');
    }).finally(() => {
      if (!controller.signal.aborted) setLoading(false);
    });
    return () => controller.abort();
  }, [isSignedIn, user?.id]);

  const setSelectedSpaceId = (id: string | null) => {
    if (id !== null && !spaces.some((space) => space.id === id)) return;
    setSelectedSpaceIdState(id);
    const userId = user?.id;
    if (!userId) return;
    const storageKey = storageKeyForUser(userId);
    if (id === null) localStorage.removeItem(storageKey);
    else localStorage.setItem(storageKey, id);
  };

  const value = useMemo(() => ({
    spaces, selectedSpaceId, setSelectedSpaceId, loading, error,
  }), [spaces, selectedSpaceId, loading, error]);
  return <SpaceContext.Provider value={value}>{children}</SpaceContext.Provider>;
}

export function useSpaceScope() {
  const value = useContext(SpaceContext);
  if (!value) throw new Error('useSpaceScope must be used within SpaceProvider');
  return value;
}

import { useEffect, useState } from 'react';
import { creditApi } from '@/lib/credit-api';

export function useCreditBalance(refreshKey: string) {
  const [balanceUsdMicros, setBalanceUsdMicros] = useState<number | null>(null);

  useEffect(() => {
    let active = true;
    let loading = false;

    const refresh = async () => {
      if (loading) return;
      loading = true;
      try {
        const account = await creditApi.account();
        if (active) setBalanceUsdMicros(account.balanceUsdMicros);
      } catch {
        // Keep the last successful balance; the indicator shows unavailable until one loads.
      } finally {
        loading = false;
      }
    };

    const refreshWhenVisible = () => {
      if (document.visibilityState === 'visible') void refresh();
    };

    void refresh();
    const interval = window.setInterval(() => void refresh(), 60_000);
    window.addEventListener('focus', refreshWhenVisible);
    document.addEventListener('visibilitychange', refreshWhenVisible);

    return () => {
      active = false;
      window.clearInterval(interval);
      window.removeEventListener('focus', refreshWhenVisible);
      document.removeEventListener('visibilitychange', refreshWhenVisible);
    };
  }, [refreshKey]);

  return balanceUsdMicros;
}
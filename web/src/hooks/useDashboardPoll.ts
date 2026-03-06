import { useCallback, useEffect, useRef, useState } from 'react';

import { fetchDashboard } from '../api/dashboard';
import type { DashboardResponse } from '../types';

const POLL_INTERVAL_MS = 30_000;

export function useDashboardPoll() {
  const [data, setData] = useState<DashboardResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdatedAt, setLastUpdatedAt] = useState<Date | null>(null);
  const abortRef = useRef<AbortController | null>(null);
  const hasLoadedRef = useRef(false);

  const refresh = useCallback(async (options?: { showLoader?: boolean }) => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    if (options?.showLoader || !hasLoadedRef.current) {
      setLoading(true);
    }

    try {
      const nextData = await fetchDashboard(controller.signal);
      hasLoadedRef.current = true;
      setData(nextData);
      setError(null);
      setLastUpdatedAt(new Date());
    } catch (cause) {
      if (controller.signal.aborted) {
        return;
      }
      const message = cause instanceof Error ? cause.message : 'unknown_dashboard_error';
      setError(message);
    } finally {
      if (!controller.signal.aborted) {
        setLoading(false);
      }
    }
  }, []);

  useEffect(() => {
    void refresh({ showLoader: true });

    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible') {
        void refresh();
      }
    }, POLL_INTERVAL_MS);

    const handleVisibilityChange = () => {
      if (document.visibilityState === 'visible') {
        void refresh();
      }
    };

    document.addEventListener('visibilitychange', handleVisibilityChange);

    return () => {
      window.clearInterval(timer);
      document.removeEventListener('visibilitychange', handleVisibilityChange);
      abortRef.current?.abort();
    };
  }, [refresh]);

  return {
    data,
    loading,
    error,
    lastUpdatedAt,
    isStale: Boolean(error && data),
    refresh,
    pollIntervalMs: POLL_INTERVAL_MS,
  };
}

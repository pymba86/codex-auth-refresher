import { useCallback, useEffect, useRef, useState } from 'react';

import { fetchDashboard } from '../api/dashboard';
import type { DashboardResponse } from '../types';

const POLL_INTERVAL_MS = 10_000;

export function useDashboardPoll() {
  const [data, setData] = useState<DashboardResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [lastUpdatedAt, setLastUpdatedAt] = useState<Date | null>(null);
  const abortRef = useRef<AbortController | null>(null);

  const refresh = useCallback(async () => {
    abortRef.current?.abort();
    const controller = new AbortController();
    abortRef.current = controller;

    if (!data) {
      setLoading(true);
    }

    try {
      const nextData = await fetchDashboard(controller.signal);
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
  }, [data]);

  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => {
      void refresh();
    }, POLL_INTERVAL_MS);

    return () => {
      window.clearInterval(timer);
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

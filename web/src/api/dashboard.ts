import type { DashboardResponse } from '../types';

export async function fetchDashboard(signal?: AbortSignal): Promise<DashboardResponse> {
  const response = await fetch('/v1/dashboard', {
    method: 'GET',
    headers: {
      Accept: 'application/json',
    },
    cache: 'no-store',
    signal,
  });

  if (!response.ok) {
    throw new Error(`dashboard_request_failed:${response.status}`);
  }

  return (await response.json()) as DashboardResponse;
}

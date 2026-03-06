export type FileState = 'ok' | 'degraded' | 'reauth_required' | 'invalid_json' | string;

export interface DashboardResponse {
  generated_at: string;
  service: {
    ready: boolean;
    started_at: string;
    uptime_seconds: number;
  };
  config: {
    refresh_before: string;
    refresh_max_age: string;
    scan_interval: string;
    max_parallel: number;
    status_api_enabled: boolean;
  };
  metrics: {
    scans_total: number;
    refresh_attempts_total: number;
    refresh_success_total: number;
    refresh_failure_total: number;
    last_scan_at?: string;
  };
  summary: {
    tracked_files: number;
    ok_files: number;
    degraded_files: number;
    reauth_required_files: number;
    invalid_json_files: number;
    disabled_files: number;
  };
  files: DashboardFile[];
}

export interface DashboardFile {
  file: string;
  account_id?: string;
  schema?: string;
  state: FileState;
  expires_at?: string;
  next_refresh_at?: string;
  last_refresh_at?: string;
  consecutive_failures: number;
  disabled: boolean;
  last_error?: string;
}

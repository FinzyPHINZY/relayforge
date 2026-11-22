export type Page<T> = { data: T[]; next_cursor: string };

export type Metrics = {
  events_accepted: number;
  outbox_pending: number;
  deliveries_pending: number;
  deliveries_retry_wait: number;
  deliveries_processing: number;
  deliveries_succeeded: number;
  deliveries_exhausted: number;
  delivery_attempts: number;
  lease_reclaims: number;
  oldest_pending_age_seconds: number;
};

export type Endpoint = {
  id: string;
  url: string;
  description: string;
  status: "active" | "paused" | "disabled";
  event_types: string[];
  timeout_ms: number;
  max_attempts: number;
  concurrency_limit: number;
  rate_limit_per_minute: number;
  next_rate_at?: string;
  signing_secrets: { id: string; state: "active" | "overlap" | "retired"; created_at: string; overlap_until?: string; retired_at?: string }[];
  circuit_status: "closed" | "open" | "half_open";
  circuit_open_until?: string;
  created_at: string;
  verification_required: boolean;
  verified_at?: string;
  field_map: Record<string, string>;
};

export type Principal = { role: "owner" | "admin" | "operator" | "viewer"; scopes: string[] };
export type TeamMember = { id: string; name: string; email: string; role: "admin" | "operator" | "viewer"; status: "active" | "suspended"; created_at: string };
export type AuditEvent = { id: string; actor_api_key_id?: string; actor_user_id?: string; actor_key_name: string; actor_role: string; action: string; target_type: "organization" | "api_key" | "team_member" | "endpoint" | "delivery" | "event"; target_id: string; details: Record<string, unknown>; created_at: string };
export type AnalyticsBucket = { start: string; attempts: number; succeeded: number; failed: number; p50_duration_ms: number | null; p95_duration_ms: number | null };
export type DeliveryAnalytics = { window: "24h" | "7d" | "30d"; buckets: AnalyticsBucket[] };
export type OverviewWindow = "24h" | "7d" | "30d";
export type OverviewBucket = {
  start: string;
  events: number;
  succeeded: number;
  exhausted: number;
  cancelled: number;
  attempts: number;
  failed_attempts: number;
  retries: number;
  p50_latency_ms: number | null;
  p95_latency_ms: number | null;
};
export type OverviewAnalytics = {
  window: OverviewWindow;
  from: string;
  to: string;
  bucket_unit: "1 hour" | "1 day";
  totals: Omit<OverviewBucket, "start" | "p50_latency_ms" | "p95_latency_ms">;
  buckets: OverviewBucket[];
};

export type Event = { id: string; type: string; status: string; created_at: string };
export type EventListItem = Event & { delivery_summary: { total: number; succeeded: number; exhausted: number; pending: number; processing: number; retry_wait: number; cancelled: number } };

export type Delivery = {
  id: string;
  event_id: string;
  endpoint_id: string;
  run_number: number;
  replayed_from_delivery_id?: string;
  destination_url: string;
  status: "pending" | "processing" | "retry_wait" | "succeeded" | "exhausted" | "cancelled";
  attempt_count: number;
  max_attempts: number;
  last_http_status?: number;
  last_error_code?: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
  next_attempt_at: string;
  review_status?: "unreviewed" | "investigating" | "resolved";
};

export type ReviewHistoryEntry = { status: "investigating" | "resolved"; note: string; api_key_id: string | null; actor_user_id: string | null; created_at: string };

export type Attempt = {
  id: string;
  attempt_number: number;
  started_at: string;
  finished_at?: string;
  duration_ms?: number;
  http_status?: number;
  outcome: string;
  error_code?: string;
  response_excerpt?: string;
  retry_scheduled_at?: string;
};

export type DeliveryDetail = { delivery: Delivery; attempts: Attempt[]; review_history: ReviewHistoryEntry[]; event_accepted_at: string; outbox_dispatched_at?: string };

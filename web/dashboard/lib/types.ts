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
  circuit_status: "closed" | "open" | "half_open";
  circuit_open_until?: string;
  created_at: string;
};

export type Event = { id: string; type: string; status: string; created_at: string; data?: unknown };

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
  completed_at?: string;
  review_status?: "unreviewed" | "investigating" | "resolved";
};

export type ReviewHistoryEntry = { status: "investigating" | "resolved"; note: string; api_key_id: string; created_at: string };

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
};

ALTER TABLE endpoints
  ADD COLUMN concurrency_limit integer NOT NULL DEFAULT 2 CHECK (concurrency_limit BETWEEN 1 AND 100),
  ADD COLUMN rate_limit_per_minute integer NOT NULL DEFAULT 0 CHECK (rate_limit_per_minute BETWEEN 0 AND 6000);

-- A claim consumes its rate slot before any HTTP call. Keeping this cursor in
-- PostgreSQL makes pacing shared by every worker and durable across restarts.
ALTER TABLE endpoint_circuits ADD COLUMN next_rate_at timestamptz;

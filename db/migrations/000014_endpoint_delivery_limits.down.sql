ALTER TABLE endpoint_circuits DROP COLUMN next_rate_at;
ALTER TABLE endpoints DROP COLUMN rate_limit_per_minute, DROP COLUMN concurrency_limit;

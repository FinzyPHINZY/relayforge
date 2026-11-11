ALTER TABLE events ADD COLUMN traceparent text CHECK (traceparent IS NULL OR length(traceparent) <= 128);
ALTER TABLE deliveries ADD COLUMN traceparent text CHECK (traceparent IS NULL OR length(traceparent) <= 128);

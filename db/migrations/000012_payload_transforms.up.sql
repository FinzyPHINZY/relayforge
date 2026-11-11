ALTER TABLE endpoints ADD COLUMN field_map jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(field_map)='object');
ALTER TABLE deliveries ADD COLUMN field_map jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(field_map)='object');

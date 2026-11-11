ALTER TABLE endpoints ADD COLUMN verification_required boolean NOT NULL DEFAULT false;
ALTER TABLE endpoints ADD COLUMN verified_at timestamptz;

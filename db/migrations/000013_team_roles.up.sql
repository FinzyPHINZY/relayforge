CREATE TABLE team_members (
  id uuid PRIMARY KEY,
  organization_id uuid NOT NULL REFERENCES organizations(id),
  name text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
  email text NOT NULL CHECK (length(email) BETWEEN 3 AND 254),
  role text NOT NULL CHECK (role IN ('admin','operator','viewer')),
  status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended')),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (organization_id,email),
  UNIQUE (id,organization_id)
);
ALTER TABLE api_keys ADD COLUMN member_id uuid;
ALTER TABLE api_keys ADD CONSTRAINT api_keys_member_org_fk FOREIGN KEY (member_id,organization_id) REFERENCES team_members(id,organization_id);

CREATE TABLE users (
    id uuid PRIMARY KEY,
    email text UNIQUE CHECK (email = lower(email) AND length(email) BETWEEN 3 AND 254),
    email_verified_at timestamptz,
    display_name text CHECK (display_name IS NULL OR length(display_name) <= 120),
    avatar_url text CHECK (avatar_url IS NULL OR length(avatar_url) <= 2048),
    kind text NOT NULL DEFAULT 'regular' CHECK (kind IN ('regular','guest')),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active','deleting','deleted')),
    created_at timestamptz NOT NULL DEFAULT now(),
    last_sign_in_at timestamptz
);

CREATE TABLE user_identities (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    provider text NOT NULL CHECK (provider IN ('github','email')),
    provider_subject text NOT NULL CHECK (length(provider_subject) BETWEEN 1 AND 320),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (provider, provider_subject)
);
CREATE INDEX user_identities_user_idx ON user_identities (user_id);

CREATE TABLE sessions (
    id uuid PRIMARY KEY,
    user_id uuid NOT NULL REFERENCES users(id),
    token_hash bytea NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_seen_at timestamptz NOT NULL DEFAULT now(),
    expires_at timestamptz NOT NULL,
    revoked_at timestamptz,
    ip_hash bytea,
    user_agent text CHECK (user_agent IS NULL OR length(user_agent) <= 256)
);
CREATE INDEX sessions_user_idx ON sessions (user_id);

CREATE TABLE magic_links (
    id uuid PRIMARY KEY,
    email text NOT NULL CHECK (email = lower(email)),
    token_hash bytea NOT NULL UNIQUE,
    purpose text NOT NULL DEFAULT 'sign_in' CHECK (purpose IN ('sign_in')),
    expires_at timestamptz NOT NULL,
    consumed_at timestamptz,
    requested_ip_hash bytea,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX magic_links_email_created_idx ON magic_links (email, created_at);

ALTER TABLE team_members ADD COLUMN user_id uuid REFERENCES users(id);
ALTER TABLE team_members ADD CONSTRAINT team_members_org_user_key UNIQUE (organization_id, user_id);
ALTER TABLE team_members DROP CONSTRAINT team_members_role_check;
ALTER TABLE team_members ADD CONSTRAINT team_members_role_check CHECK (role IN ('owner','admin','operator','viewer'));

ALTER TABLE organizations ADD COLUMN plan text NOT NULL DEFAULT 'free' CHECK (plan IN ('free'));
ALTER TABLE organizations ADD COLUMN created_by_user_id uuid REFERENCES users(id);

-- api_keys.name and last_used_at already exist (000001); only bound the name for new rows.
ALTER TABLE api_keys ADD CONSTRAINT api_keys_name_length_check CHECK (length(name) BETWEEN 1 AND 80) NOT VALID;
ALTER TABLE api_keys ADD COLUMN role text CHECK (role IN ('owner','admin','operator','viewer'));
ALTER TABLE api_keys ADD COLUMN created_by_user_id uuid REFERENCES users(id);

ALTER TABLE audit_events ADD COLUMN actor_user_id uuid;
ALTER TABLE audit_events DROP CONSTRAINT audit_events_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_actor_check CHECK (
    (actor_role='system' AND actor_api_key_id IS NULL AND actor_user_id IS NULL)
    OR (actor_role<>'system' AND (actor_api_key_id IS NOT NULL OR actor_user_id IS NOT NULL))
);
ALTER TABLE audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_action_check CHECK (action IN (
    'organization.created','organization.suspended','api_key.issued',
    'team.member_created','team.member_updated',
    'endpoint.created','endpoint.updated','endpoint.disabled','endpoint.verified','endpoint.secret_rotated',
    'delivery.replayed','delivery.reviewed','event.payload_viewed',
    'user.signed_in','user.signed_out','session.revoked','api_key.revoked','organization.created_by_user'
));
ALTER TABLE audit_events DROP CONSTRAINT audit_events_target_type_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_target_type_check CHECK (target_type IN ('organization','api_key','team_member','endpoint','delivery','event','user','session'));

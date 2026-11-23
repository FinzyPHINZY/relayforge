-- Audit rows that only the new schema can express are purged (explicit opt-in
-- to the append-only guard), since the restored CHECKs could not hold them.
SELECT set_config('relayforge.audit_purge','on',false);
DELETE FROM audit_events
WHERE actor_user_id IS NOT NULL
   OR action IN ('user.signed_in','user.signed_out','session.revoked','api_key.revoked','organization.created_by_user')
   OR target_type IN ('user','session');
SELECT set_config('relayforge.audit_purge','off',false);

ALTER TABLE audit_events DROP CONSTRAINT audit_events_target_type_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_target_type_check CHECK (target_type IN ('organization','api_key','team_member','endpoint','delivery','event'));
ALTER TABLE audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_action_check CHECK (action IN (
    'organization.created','organization.suspended','api_key.issued',
    'team.member_created','team.member_updated',
    'endpoint.created','endpoint.updated','endpoint.disabled','endpoint.verified','endpoint.secret_rotated',
    'delivery.replayed','delivery.reviewed','event.payload_viewed'
));
ALTER TABLE audit_events DROP CONSTRAINT audit_events_actor_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_check CHECK ((actor_role='system' AND actor_api_key_id IS NULL) OR (actor_role<>'system' AND actor_api_key_id IS NOT NULL));
ALTER TABLE audit_events DROP COLUMN actor_user_id;

-- The pre-accounts binary resolves a key's role as COALESCE(member.role,'owner'),
-- so a console-created key (no member, role stored on the key) below owner would
-- silently become owner once api_keys.role is gone. Revoke those keys instead.
UPDATE api_keys SET revoked_at=now()
WHERE member_id IS NULL AND role IS NOT NULL AND role<>'owner' AND revoked_at IS NULL;

ALTER TABLE api_keys DROP COLUMN created_by_user_id;
ALTER TABLE api_keys DROP COLUMN role;
ALTER TABLE api_keys DROP CONSTRAINT api_keys_name_length_check;

ALTER TABLE organizations DROP COLUMN created_by_user_id;
ALTER TABLE organizations DROP COLUMN plan;

-- Owner members and user-linked members cannot exist in the old schema.
UPDATE team_members SET role='admin' WHERE role='owner';
ALTER TABLE team_members DROP CONSTRAINT team_members_role_check;
ALTER TABLE team_members ADD CONSTRAINT team_members_role_check CHECK (role IN ('admin','operator','viewer'));
ALTER TABLE team_members DROP CONSTRAINT team_members_org_user_key;
ALTER TABLE team_members DROP COLUMN user_id;

DROP TABLE magic_links;
DROP TABLE sessions;
DROP TABLE user_identities;
DROP TABLE users;

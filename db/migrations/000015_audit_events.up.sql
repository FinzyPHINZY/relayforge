CREATE TABLE audit_events (
    id uuid PRIMARY KEY,
    organization_id uuid NOT NULL REFERENCES organizations(id),
    actor_api_key_id uuid,
    actor_role text NOT NULL CHECK (actor_role IN ('system','owner','admin','operator','viewer')),
    action text NOT NULL CHECK (action IN (
        'organization.created','organization.suspended','api_key.issued',
        'team.member_created','team.member_updated',
        'endpoint.created','endpoint.updated','endpoint.disabled','endpoint.verified','endpoint.secret_rotated',
        'delivery.replayed','delivery.reviewed'
    )),
    target_type text NOT NULL CHECK (target_type IN ('organization','api_key','team_member','endpoint','delivery')),
    target_id uuid NOT NULL,
    details jsonb NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(details)='object' AND octet_length(details::text)<=4096),
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    CHECK ((actor_role='system' AND actor_api_key_id IS NULL) OR (actor_role<>'system' AND actor_api_key_id IS NOT NULL))
);
CREATE INDEX audit_events_org_time_idx ON audit_events (organization_id,created_at DESC,id DESC);
CREATE INDEX audit_events_target_idx ON audit_events (organization_id,target_type,target_id,created_at DESC,id DESC);

-- Audit records are immutable during normal operation. Tenant purge explicitly
-- opts in to deletion after a complete export has been verified.
CREATE FUNCTION guard_audit_events() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP='DELETE' AND current_setting('relayforge.audit_purge',true)='on' THEN
        RETURN OLD;
    END IF;
    RAISE EXCEPTION 'audit events are append-only';
END;
$$;
CREATE TRIGGER audit_events_guard BEFORE UPDATE OR DELETE ON audit_events
FOR EACH ROW EXECUTE FUNCTION guard_audit_events();

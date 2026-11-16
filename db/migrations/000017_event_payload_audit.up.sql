ALTER TABLE audit_events DROP CONSTRAINT audit_events_action_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_action_check CHECK (action IN (
    'organization.created','organization.suspended','api_key.issued',
    'team.member_created','team.member_updated',
    'endpoint.created','endpoint.updated','endpoint.disabled','endpoint.verified','endpoint.secret_rotated',
    'delivery.replayed','delivery.reviewed','event.payload_viewed'
));
ALTER TABLE audit_events DROP CONSTRAINT audit_events_target_type_check;
ALTER TABLE audit_events ADD CONSTRAINT audit_events_target_type_check CHECK (target_type IN ('organization','api_key','team_member','endpoint','delivery','event'));

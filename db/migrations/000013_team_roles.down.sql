ALTER TABLE api_keys DROP CONSTRAINT api_keys_member_org_fk;
ALTER TABLE api_keys DROP COLUMN member_id;
DROP TABLE team_members;

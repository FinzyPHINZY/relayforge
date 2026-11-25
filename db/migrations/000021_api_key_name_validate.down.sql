-- normalised names cannot be restored; the check returns to its NOT VALID state.
ALTER TABLE api_keys DROP CONSTRAINT api_keys_name_length_check;
ALTER TABLE api_keys ADD CONSTRAINT api_keys_name_length_check CHECK (length(name) BETWEEN 1 AND 80) NOT VALID;

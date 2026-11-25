-- rows written before 000019 may break api_keys_name_length_check, and postgresql
-- re-checks a NOT VALID constraint on every UPDATE of such a row, so revoking them
-- would fail. normalise those names, then validate.
UPDATE api_keys SET name = CASE
    WHEN btrim(name, E' \t\r\n') = '' THEN 'Legacy key'
    ELSE btrim(left(btrim(name, E' \t\r\n'), 80), E' \t\r\n')
END
WHERE name <> btrim(name, E' \t\r\n') OR length(name) NOT BETWEEN 1 AND 80;
ALTER TABLE api_keys VALIDATE CONSTRAINT api_keys_name_length_check;

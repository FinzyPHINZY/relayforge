CREATE TABLE endpoint_circuits (
    endpoint_id uuid PRIMARY KEY,
    organization_id uuid NOT NULL,
    failure_streak integer NOT NULL DEFAULT 0 CHECK (failure_streak >= 0),
    open_until timestamptz,
    probe_delivery_id uuid,
    updated_at timestamptz NOT NULL DEFAULT now(),
    FOREIGN KEY (endpoint_id, organization_id) REFERENCES endpoints(id, organization_id),
    CHECK (probe_delivery_id IS NULL OR open_until IS NOT NULL)
);
INSERT INTO endpoint_circuits (endpoint_id, organization_id) SELECT id, organization_id FROM endpoints;

CREATE FUNCTION create_endpoint_circuit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO endpoint_circuits (endpoint_id, organization_id) VALUES (NEW.id, NEW.organization_id);
    RETURN NEW;
END $$;
CREATE TRIGGER endpoint_circuit_insert AFTER INSERT ON endpoints FOR EACH ROW EXECUTE FUNCTION create_endpoint_circuit();

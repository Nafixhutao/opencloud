-- Phase 4D makes deployment progress durable. The existing immutable-identity
-- trigger remains responsible for image/revision fields; this trigger prevents
-- a worker retry or an accidental query from skipping lifecycle guarantees.
CREATE FUNCTION enforce_opencloud_deployment_lifecycle() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.status = OLD.status THEN
        RETURN NEW;
    END IF;

    IF (OLD.status = 'queued' AND NEW.status IN (
            'cloning', 'detecting', 'planning', 'building', 'pushing',
            'failed', 'cancelled'
        )) OR
       (OLD.status = 'cloning' AND NEW.status IN (
            'detecting', 'planning', 'building', 'pushing', 'failed', 'cancelled'
        )) OR
       (OLD.status = 'detecting' AND NEW.status IN (
            'planning', 'building', 'pushing', 'failed', 'cancelled'
        )) OR
       (OLD.status = 'planning' AND NEW.status IN (
            'building', 'pushing', 'failed', 'cancelled'
        )) OR
       (OLD.status = 'building' AND NEW.status IN (
            'pushing', 'failed', 'cancelled'
        )) OR
       (OLD.status = 'pushing' AND NEW.status IN (
            'scanning', 'deploying', 'failed', 'cancelled'
        )) OR
       (OLD.status = 'scanning' AND NEW.status IN (
            'deploying', 'failed', 'cancelled'
        )) OR
       (OLD.status = 'deploying' AND NEW.status IN (
            'ready', 'failed', 'cancelled'
        )) THEN
        RETURN NEW;
    END IF;

    RAISE EXCEPTION 'invalid deployment lifecycle transition: % -> %', OLD.status, NEW.status;
END;
$$;

CREATE TRIGGER deployments_lifecycle_guard
BEFORE UPDATE OF status ON deployments
FOR EACH ROW
EXECUTE FUNCTION enforce_opencloud_deployment_lifecycle();

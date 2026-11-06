-- The audit records as a data class (assembly protocol 11, decision A158): one key column for the composite
-- (source, id), written with every record (a generated column is not there yet when the write barrier's BEFORE
-- trigger runs), checked to say the same.
ALTER TABLE audit_record ADD COLUMN rkey text;
UPDATE audit_record SET rkey = source || '/' || id::text;
ALTER TABLE audit_record ALTER COLUMN rkey SET NOT NULL;
ALTER TABLE audit_record ADD CONSTRAINT audit_record_rkey_is_the_key CHECK (rkey = source || '/' || id::text);
CREATE UNIQUE INDEX audit_record_rkey ON audit_record (rkey);

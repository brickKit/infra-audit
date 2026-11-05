-- The evidence store the audit component also serves (assembly protocol 15; component protocol P12.9-P12.14;
-- contract/README.md, EvidenceService): records kept once by (id, phase), apart from the audit records, never rolled
-- back with a full restore. The store's sequence goes on across epochs (a receipt is its epoch and sequence).
CREATE TABLE audit_evidence (
    id          uuid NOT NULL,
    phase       text NOT NULL CHECK (phase IN ('intent', 'effective', 'cancelled', 'general')),
    recorder    text NOT NULL,                      -- the recording component (its certificate)
    category    text NOT NULL,
    digest      text NOT NULL,
    record      bytea NOT NULL,                     -- be.evidence.record, canonical, as received
    epoch       text NOT NULL,                      -- the audit database's epoch at storage
    seq         bigint NOT NULL UNIQUE,
    received_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, phase)
);
CREATE INDEX audit_evidence_recorder ON audit_evidence (recorder, seq);

CREATE TABLE audit_evidence_counter (
    single   boolean PRIMARY KEY DEFAULT true CHECK (single),
    last_seq bigint NOT NULL DEFAULT 0
);
INSERT INTO audit_evidence_counter (single) VALUES (true);

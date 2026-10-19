-- The audit component's store (protocol 03; contract/README.md): records as their canonical bytes, by source and
-- ID, each with the audit database's epoch and storage sequence and the source's chain digest.
CREATE TABLE audit_record (
    source       text NOT NULL,                      -- the source component (its certificate)
    id           uuid NOT NULL,
    digest       text NOT NULL,
    record       bytea NOT NULL,                     -- be.audit.record, canonical, as received
    source_seq   bigint NOT NULL,                    -- the source's production sequence
    source_epoch text NOT NULL,
    epoch        text NOT NULL,                      -- the audit database's epoch at storage
    storage_seq  bigint NOT NULL,
    chain        text NOT NULL,
    received_at  timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source, id),
    UNIQUE (epoch, storage_seq)
);
CREATE INDEX audit_record_received ON audit_record (received_at DESC);

-- The storage sequence of the current epoch, under the lock every intake takes.
CREATE TABLE audit_counter (
    single   boolean PRIMARY KEY DEFAULT true CHECK (single),
    epoch    text,
    last_seq bigint NOT NULL DEFAULT 0
);
INSERT INTO audit_counter (single) VALUES (true);

-- Epoch history (protocol 14 rule 2): an older epoch and the last storage sequence it kept (its fork point).
CREATE TABLE audit_epoch (
    epoch      text PRIMARY KEY,
    fork       bigint NOT NULL,
    closed_at  timestamptz NOT NULL DEFAULT now()
);

-- Per source: the chain so far and the last production sequence seen in the source's epoch (to report gaps).
CREATE TABLE audit_source (
    source       text PRIMARY KEY,
    chain        text NOT NULL,
    source_epoch text NOT NULL,
    last_seq     bigint NOT NULL
);

-- Completed backups (protocol 15): the highest storage sequence of an epoch a backup contains. How a backup is
-- registered comes with protocol 15's implementation; until then this stays empty and nothing is deletable.
CREATE TABLE audit_backup (
    epoch        text NOT NULL,
    seq          bigint NOT NULL,
    completed_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (epoch, seq)
);

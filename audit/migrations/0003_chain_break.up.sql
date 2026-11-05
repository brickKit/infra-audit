-- Foundations item 4, T4e (protocol 03, AU14): a source's chain that does not verify, as the chain check found it:
-- the first record where it breaks (or the head that does not match). An integrity event, for a person to examine.
CREATE TABLE audit_chain_break (
    source   text NOT NULL,
    at_epoch text NOT NULL,
    at_seq   bigint NOT NULL,
    detail   text NOT NULL,
    found_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source, at_epoch, at_seq)
);

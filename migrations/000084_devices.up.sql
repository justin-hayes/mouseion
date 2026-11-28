-- Devices (#2869, ADR 0143). A Device is a named credential the learner issues
-- for a device protocol such as KOReader's kosync, so the device never holds
-- the account password. username is unique across the installation and
-- key_hash is the SHA-256 of the MD5 hex key KOReader sends; the generated
-- password itself is never stored. last_seen_at is written by the kosync
-- surface and revoked_at by revocation; both stay NULL until then.
--
-- Additive structural DDL only: a new table, no change to an existing one and
-- no data backfill. Forward-only in production (ADR 0038, ADR 0070).
CREATE TABLE public.devices (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    name text NOT NULL,
    username text NOT NULL,
    key_hash text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    last_seen_at timestamptz,
    revoked_at timestamptz,
    CONSTRAINT devices_pkey PRIMARY KEY (id),
    CONSTRAINT devices_username_key UNIQUE (username),
    CONSTRAINT devices_name_check CHECK (name <> ''),
    CONSTRAINT devices_username_check CHECK (username <> ''),
    CONSTRAINT devices_key_hash_check CHECK (key_hash <> ''),
    CONSTRAINT devices_owner_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE
);
CREATE INDEX devices_owner_idx ON public.devices (owner_id, created_at);

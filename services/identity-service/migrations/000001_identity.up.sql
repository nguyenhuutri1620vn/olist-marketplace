-- Accounts (FR14) and the RS256 signing key (AD-14).
CREATE TABLE account (
    id            uuid PRIMARY KEY,
    -- Stored trimmed and lower-cased; the check keeps every writer honest.
    email         text NOT NULL CHECK (email = lower(btrim(email)) AND email <> ''),
    password_hash text NOT NULL,
    role          text NOT NULL CHECK (role IN ('customer', 'seller', 'admin')),
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX account_email_key ON account (email);

-- PKCS#8 DER private keys. Only one is ever created; the newest signs and is
-- the only one in the JWKS (key rotation is not supported yet).
CREATE TABLE signing_key (
    kid         text PRIMARY KEY,
    private_key bytea NOT NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- migrations/001_init.sql
-- Initial schema: roles, users, audit_logs
-- PostgreSQL 13+

-- Enable UUID generation extension
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

--------------------------------------------------------------------
-- 1. Roles table
--------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS roles (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    name TEXT NOT NULL UNIQUE,               -- e.g. 'admin', 'accountant', 'viewer'
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

--------------------------------------------------------------------
-- 2. Users table
--------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,             -- bcrypt/argon2 hash
    role_id UUID NOT NULL REFERENCES roles(id) ON DELETE RESTRICT,
    is_active BOOLEAN NOT NULL DEFAULT true,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ                     -- soft‑delete timestamp
);

-- Indexes for fast look‑ups
CREATE INDEX IF NOT EXISTS idx_users_email   ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_role_id ON users(role_id);

--------------------------------------------------------------------
-- 3. Audit logs table
--------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,                     -- e.g. 'upload', 'validation', 'webhook_sent'
    entity_type TEXT,                        -- e.g. 'invoice', 'document'
    entity_id UUID,                           -- FK to the affected entity (optional)
    details JSONB,                           -- arbitrary key/value details
    ip_address INET,                         -- source IP of the request
    user_agent TEXT,                          -- raw user‑agent string
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Indexes for audit queries
CREATE INDEX IF NOT EXISTS idx_audit_user      ON audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_entity    ON audit_logs(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_audit_created   ON audit_logs(created_at);

--------------------------------------------------------------------
-- 4. Timestamp trigger (auto‑update updated_at)
--------------------------------------------------------------------
CREATE OR REPLACE FUNCTION set_updated_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Attach trigger to tables that have an updated_at column
CREATE TRIGGER trg_roles_updated
BEFORE UPDATE ON roles
FOR EACH ROW EXECUTE FUNCTION set_updated_timestamp();

CREATE TRIGGER trg_users_updated
BEFORE UPDATE ON users
FOR EACH ROW EXECUTE FUNCTION set_updated_timestamp();

--------------------------------------------------------------------
-- 5. Seed default roles (idempotent)
--------------------------------------------------------------------
INSERT INTO roles (name, description)
VALUES
    ('admin',      'Full access to all resources and settings'),
    ('accountant', 'Can view, upload and validate invoices, manage webhooks'),
    ('viewer',     'Read‑only access to dashboards and audit logs')
ON CONFLICT (name) DO NOTHING;

--------------------------------------------------------------------
-- End of 001_init.sql
--------------------------------------------------------------------
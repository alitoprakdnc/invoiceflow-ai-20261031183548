-- migrations/002_invoice_table.sql
-- ------------------------------------------------------------
-- Migration: Create core tables for invoice processing workflow
-- Includes: invoices, documents, extraction_results, webhook_endpoints
-- ------------------------------------------------------------

-- Enable the pgcrypto extension for UUID generation (if not already enabled)
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

--------------------------------------------------------------------
-- Table: invoices
--------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS invoices (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL,
    vendor_name       TEXT,
    invoice_number    TEXT,
    invoice_date      DATE,
    total_amount      NUMERIC(12, 2),
    currency          CHAR(3) DEFAULT 'USD',
    status            VARCHAR(20) NOT NULL DEFAULT 'pending',
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at        TIMESTAMPTZ,

    CONSTRAINT fk_invoices_user
        FOREIGN KEY (user_id)
        REFERENCES users (id)
        ON DELETE CASCADE
);

-- Indexes for fast look‑ups
CREATE INDEX IF NOT EXISTS idx_invoices_user_id   ON invoices (user_id);
CREATE INDEX IF NOT EXISTS idx_invoices_status    ON invoices (status);
CREATE INDEX IF NOT EXISTS idx_invoices_created_at ON invoices (created_at);

--------------------------------------------------------------------
-- Table: documents
--------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS documents (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    invoice_id        UUID NOT NULL,
    file_name         TEXT NOT NULL,
    content_type      TEXT NOT NULL,
    storage_path      TEXT NOT NULL,
    file_size_bytes   BIGINT,
    uploaded_at       TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_documents_invoice
        FOREIGN KEY (invoice_id)
        REFERENCES invoices (id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_documents_invoice_id ON documents (invoice_id);
CREATE INDEX IF NOT EXISTS idx_documents_uploaded_at ON documents (uploaded_at);

--------------------------------------------------------------------
-- Table: extraction_results
--------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS extraction_results (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    document_id       UUID NOT NULL,
    extracted_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    raw_text          TEXT,
    result_json       JSONB NOT NULL,
    validation_status VARCHAR(20) NOT NULL DEFAULT 'unvalidated',
    validation_errors JSONB,

    CONSTRAINT fk_extraction_document
        FOREIGN KEY (document_id)
        REFERENCES documents (id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_extraction_document_id ON extraction_results (document_id);
CREATE INDEX IF NOT EXISTS idx_extraction_status      ON extraction_results (validation_status);
CREATE INDEX IF NOT EXISTS idx_extraction_extracted_at ON extraction_results (extracted_at);

--------------------------------------------------------------------
-- Table: webhook_endpoints
--------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS webhook_endpoints (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id           UUID NOT NULL,
    url               TEXT NOT NULL,
    secret            TEXT NOT NULL,               -- HMAC secret stored securely
    event_type        VARCHAR(30) NOT NULL DEFAULT 'invoice_processed',
    enabled           BOOLEAN NOT NULL DEFAULT true,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT fk_webhook_user
        FOREIGN KEY (user_id)
        REFERENCES users (id)
        ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_webhook_user_id   ON webhook_endpoints (user_id);
CREATE INDEX IF NOT EXISTS idx_webhook_event_type ON webhook_endpoints (event_type);
CREATE INDEX IF NOT EXISTS idx_webhook_enabled   ON webhook_endpoints (enabled);

--------------------------------------------------------------------
-- Triggers to keep `updated_at` in sync
--------------------------------------------------------------------
-- invoices
CREATE OR REPLACE FUNCTION trigger_set_timestamp()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS set_timestamp_invoices ON invoices;
CREATE TRIGGER set_timestamp_invoices
BEFORE UPDATE ON invoices
FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

-- webhook_endpoints
DROP TRIGGER IF EXISTS set_timestamp_webhooks ON webhook_endpoints;
CREATE TRIGGER set_timestamp_webhooks
BEFORE UPDATE ON webhook_endpoints
FOR EACH ROW EXECUTE FUNCTION trigger_set_timestamp();

--------------------------------------------------------------------
-- End of migration
--------------------------------------------------------------------
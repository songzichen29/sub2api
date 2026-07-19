ALTER TABLE payment_orders
    ADD COLUMN IF NOT EXISTS invoice_status VARCHAR(20) NOT NULL DEFAULT 'UNAPPLIED',
    ADD COLUMN IF NOT EXISTS invoice_application_id BIGINT;

CREATE INDEX IF NOT EXISTS idx_payment_orders_invoice_status ON payment_orders (invoice_status);
CREATE INDEX IF NOT EXISTS idx_payment_orders_invoice_application_id ON payment_orders (invoice_application_id);

CREATE TABLE IF NOT EXISTS invoice_settings (
    id BIGSERIAL PRIMARY KEY,
    min_amount DECIMAL(20,2) NOT NULL DEFAULT 300.00,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO invoice_settings (id, min_amount)
VALUES (1, 300.00)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS invoice_headers (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    title_type VARCHAR(20) NOT NULL DEFAULT 'personal',
    title VARCHAR(255) NOT NULL,
    tax_number VARCHAR(64) NOT NULL DEFAULT '',
    email VARCHAR(255) NOT NULL DEFAULT '',
    phone VARCHAR(32) NOT NULL DEFAULT '',
    address TEXT,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_invoice_headers_user_id ON invoice_headers (user_id);

CREATE TABLE IF NOT EXISTS invoice_applications (
    id BIGSERIAL PRIMARY KEY,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    invoice_type VARCHAR(20) NOT NULL DEFAULT 'ordinary',
    header_type VARCHAR(20) NOT NULL,
    header_title VARCHAR(255) NOT NULL,
    header_tax_number VARCHAR(64) NOT NULL DEFAULT '',
    header_email VARCHAR(255) NOT NULL DEFAULT '',
    header_phone VARCHAR(32) NOT NULL DEFAULT '',
    header_address TEXT,
    total_amount DECIMAL(20,2) NOT NULL,
    handled_by BIGINT,
    rejection_reason TEXT,
    admin_note TEXT,
    invoice_number VARCHAR(128) NOT NULL DEFAULT '',
    processed_at TIMESTAMPTZ,
    invoiced_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_invoice_applications_user_status ON invoice_applications (user_id, status);
CREATE INDEX IF NOT EXISTS idx_invoice_applications_created_at ON invoice_applications (created_at);

CREATE TABLE IF NOT EXISTS invoice_application_orders (
    id BIGSERIAL PRIMARY KEY,
    application_id BIGINT NOT NULL REFERENCES invoice_applications(id) ON DELETE CASCADE,
    order_id BIGINT NOT NULL REFERENCES payment_orders(id) ON DELETE RESTRICT,
    order_no VARCHAR(128) NOT NULL DEFAULT '',
    order_type VARCHAR(20) NOT NULL DEFAULT '',
    amount DECIMAL(20,2) NOT NULL,
    paid_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT invoice_application_orders_application_order_key UNIQUE (application_id, order_id)
);
CREATE INDEX IF NOT EXISTS idx_invoice_application_orders_order_id ON invoice_application_orders (order_id);

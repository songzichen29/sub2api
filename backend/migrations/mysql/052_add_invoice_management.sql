SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'payment_orders' AND column_name = 'invoice_status'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `payment_orders` ADD COLUMN `invoice_status` VARCHAR(20) NOT NULL DEFAULT ''UNAPPLIED''',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @col_exists := (
    SELECT COUNT(*) FROM information_schema.columns
    WHERE table_schema = DATABASE() AND table_name = 'payment_orders' AND column_name = 'invoice_application_id'
);
SET @ddl := IF(@col_exists = 0,
    'ALTER TABLE `payment_orders` ADD COLUMN `invoice_application_id` BIGINT NULL',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists := (
    SELECT COUNT(*) FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'payment_orders' AND index_name = 'idx_payment_orders_invoice_status'
);
SET @ddl := IF(@idx_exists = 0,
    'CREATE INDEX `idx_payment_orders_invoice_status` ON `payment_orders` (`invoice_status`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

SET @idx_exists := (
    SELECT COUNT(*) FROM information_schema.statistics
    WHERE table_schema = DATABASE() AND table_name = 'payment_orders' AND index_name = 'idx_payment_orders_invoice_application_id'
);
SET @ddl := IF(@idx_exists = 0,
    'CREATE INDEX `idx_payment_orders_invoice_application_id` ON `payment_orders` (`invoice_application_id`)',
    'SELECT 1'
);
PREPARE stmt FROM @ddl;
EXECUTE stmt;
DEALLOCATE PREPARE stmt;

CREATE TABLE IF NOT EXISTS invoice_settings (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    min_amount DECIMAL(20,2) NOT NULL DEFAULT 300.00,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6)
);

INSERT IGNORE INTO invoice_settings (id, min_amount) VALUES (1, 300.00);

CREATE TABLE IF NOT EXISTS invoice_headers (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT NOT NULL,
    title_type VARCHAR(20) NOT NULL DEFAULT 'personal',
    title VARCHAR(255) NOT NULL,
    tax_number VARCHAR(64) NOT NULL DEFAULT '',
    email VARCHAR(255) NOT NULL DEFAULT '',
    phone VARCHAR(32) NOT NULL DEFAULT '',
    address LONGTEXT NULL,
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    CONSTRAINT fk_invoice_headers_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE,
    INDEX idx_invoice_headers_user_id (user_id)
);

CREATE TABLE IF NOT EXISTS invoice_applications (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT NOT NULL,
    status VARCHAR(20) NOT NULL DEFAULT 'PENDING',
    invoice_type VARCHAR(20) NOT NULL DEFAULT 'ordinary',
    header_type VARCHAR(20) NOT NULL,
    header_title VARCHAR(255) NOT NULL,
    header_tax_number VARCHAR(64) NOT NULL DEFAULT '',
    header_email VARCHAR(255) NOT NULL DEFAULT '',
    header_phone VARCHAR(32) NOT NULL DEFAULT '',
    header_address LONGTEXT NULL,
    total_amount DECIMAL(20,2) NOT NULL,
    handled_by BIGINT NULL,
    rejection_reason LONGTEXT NULL,
    admin_note LONGTEXT NULL,
    invoice_number VARCHAR(128) NOT NULL DEFAULT '',
    processed_at DATETIME(6) NULL,
    invoiced_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    updated_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6) ON UPDATE CURRENT_TIMESTAMP(6),
    CONSTRAINT fk_invoice_applications_user FOREIGN KEY (user_id) REFERENCES users(id),
    INDEX idx_invoice_applications_user_status (user_id, status),
    INDEX idx_invoice_applications_created_at (created_at)
);

CREATE TABLE IF NOT EXISTS invoice_application_orders (
    id BIGINT PRIMARY KEY AUTO_INCREMENT,
    application_id BIGINT NOT NULL,
    order_id BIGINT NOT NULL,
    order_no VARCHAR(128) NOT NULL DEFAULT '',
    order_type VARCHAR(20) NOT NULL DEFAULT '',
    amount DECIMAL(20,2) NOT NULL,
    paid_at DATETIME(6) NULL,
    created_at DATETIME(6) NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
    CONSTRAINT fk_invoice_application_orders_application FOREIGN KEY (application_id) REFERENCES invoice_applications(id) ON DELETE CASCADE,
    CONSTRAINT fk_invoice_application_orders_order FOREIGN KEY (order_id) REFERENCES payment_orders(id),
    CONSTRAINT uq_invoice_application_orders_application_order UNIQUE (application_id, order_id),
    INDEX idx_invoice_application_orders_order_id (order_id)
);

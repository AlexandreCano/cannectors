INSERT INTO source_customers (id, external_id, email, full_name, status, phone, updated_at, created_at, metadata) VALUES
    (1, 'CUST-001', 'ada.lovelace@example.test', 'Ada Lovelace', 'active', '+1-555-0101', '2026-01-01T10:00:00Z', '2025-12-01T08:00:00Z', '{"plan":"pro","source":"erp"}'),
    (2, 'CUST-002', 'grace.hopper@example.test', 'Grace Hopper', 'active', NULL, '2026-01-01T10:05:00Z', '2025-12-02T08:00:00Z', '{"plan":"enterprise","source":"crm"}'),
    (3, 'CUST-003', NULL, 'Null Email Customer', 'pending', '+1-555-0103', '2026-01-01T10:10:00Z', '2025-12-03T08:00:00Z', '{"scenario":"missing-email"}'),
    (4, 'CUST-004', 'duplicate@example.test', 'Duplicate Email Candidate', 'active', '+1-555-0104', '2026-01-01T10:15:00Z', '2025-12-04T08:00:00Z', '{"scenario":"dest-unique-violation"}'),
    (5, 'CUST-005', 'inactive@example.test', 'Inactive Customer', 'inactive', '+1-555-0105', '2026-01-01T10:20:00Z', '2025-12-05T08:00:00Z', '{"scenario":"filter-out"}'),
    (6, 'CUST-006', 'pagination.tail@example.test', 'Pagination Tail', 'active', '+1-555-0106', '2026-01-01T10:25:00Z', '2025-12-06T08:00:00Z', '{"scenario":"pagination"}');

INSERT INTO source_orders (id, external_order_id, customer_external_id, product_sku, quantity, total_amount, status, updated_at, created_at, metadata) VALUES
    (1001, 'ORD-1001', 'CUST-001', 'SKU-001', 2, 39.98, 'paid', '2026-01-02T09:00:00Z', '2026-01-02T08:55:00Z', '{"channel":"web"}'),
    (1002, 'ORD-1002', 'CUST-002', 'SKU-002', 1, 149.00, 'paid', '2026-01-02T09:05:00Z', '2026-01-02T09:00:00Z', '{"channel":"partner"}'),
    (1003, 'ORD-1003', 'CUST-003', 'SKU-003', NULL, NULL, 'draft', '2026-01-02T09:10:00Z', '2026-01-02T09:05:00Z', '{"scenario":"optional-fields"}'),
    (1004, 'ORD-DUPLICATE', 'CUST-004', 'SKU-001', 1, 19.99, 'paid', '2026-01-02T09:15:00Z', '2026-01-02T09:10:00Z', '{"scenario":"dest-unique-violation"}'),
    (1005, 'ORD-1005', 'CUST-999', 'SKU-404', 3, 27.00, 'paid', '2026-01-02T09:20:00Z', '2026-01-02T09:15:00Z', '{"scenario":"missing-reference"}'),
    (1006, 'ORD-1006', 'CUST-006', 'SKU-002', 0, 0.00, 'cancelled', '2026-01-02T09:25:00Z', '2026-01-02T09:20:00Z', '{"scenario":"controlled-check-violation"}');

INSERT INTO source_inventory (id, sku, warehouse_code, quantity_available, reorder_point, updated_at, metadata) VALUES
    (2001, 'SKU-001', 'east', 12, 5, '2026-01-03T08:00:00Z', '{"bin":"A-01"}'),
    (2002, 'SKU-002', 'east', 0, 10, '2026-01-03T08:05:00Z', '{"bin":"A-02","scenario":"out-of-stock"}'),
    (2003, 'SKU-003', 'west', NULL, 2, '2026-01-03T08:10:00Z', '{"scenario":"missing-quantity"}'),
    (2004, 'SKU-004', 'west', 50, 20, '2026-01-03T08:15:00Z', '{"scenario":"pagination"}'),
    (2005, 'SKU-BROKEN', 'east', -5, 1, '2026-01-03T08:20:00Z', '{"scenario":"controlled-check-violation"}'),
    (2006, 'SKU-002', 'central', 7, 3, '2026-01-03T08:25:00Z', '{"scenario":"multi-warehouse"}');

INSERT INTO customer_reference (external_id, segment, region, active, updated_at) VALUES
    ('CUST-001', 'premium', 'north-america', true, '2026-01-04T07:00:00Z'),
    ('CUST-002', 'enterprise', 'north-america', true, '2026-01-04T07:05:00Z'),
    ('CUST-003', 'trial', 'emea', true, '2026-01-04T07:10:00Z'),
    ('CUST-004', 'standard', 'emea', true, '2026-01-04T07:15:00Z'),
    ('CUST-005', 'standard', 'apac', false, '2026-01-04T07:20:00Z'),
    ('CUST-006', 'standard', 'latam', true, '2026-01-04T07:25:00Z');

INSERT INTO product_reference (sku, product_name, category, unit_price, active, updated_at) VALUES
    ('SKU-001', 'Local Widget', 'widgets', 19.99, true, '2026-01-04T08:00:00Z'),
    ('SKU-002', 'Enterprise Gadget', 'gadgets', 149.00, true, '2026-01-04T08:05:00Z'),
    ('SKU-003', 'Legacy Adapter', 'adapters', 9.00, false, '2026-01-04T08:10:00Z'),
    ('SKU-004', 'Bulk Connector', 'connectors', 3.50, true, '2026-01-04T08:15:00Z');

INSERT INTO dest_customers (external_id, email, full_name, segment, source_updated_at, synced_at) VALUES
    ('CUST-DEST-001', 'duplicate@example.test', 'Existing Duplicate Email', 'legacy', '2025-12-15T12:00:00Z', '2026-01-05T09:00:00Z');

INSERT INTO dest_orders (external_order_id, customer_external_id, product_sku, quantity, total_amount, status, synced_at) VALUES
    ('ORD-DUPLICATE', 'CUST-DEST-001', 'SKU-001', 1, 19.99, 'paid', '2026-01-05T09:05:00Z');

INSERT INTO inventory_snapshot (sku, warehouse_code, quantity_available, snapshot_at, sync_source) VALUES
    ('SKU-001', 'east', 10, '2026-01-05T09:10:00Z', 'seed');

-- Truncate all test-lab tables and reseed from 002_seed.sql.
-- This file is kept minimal to avoid duplicating seed data.
-- Usage: cat reset.sql 002_seed.sql | psql ...
TRUNCATE TABLE
    inventory_snapshot,
    dest_orders,
    dest_customers,
    product_reference,
    customer_reference,
    source_inventory,
    source_orders,
    source_customers
RESTART IDENTITY CASCADE;

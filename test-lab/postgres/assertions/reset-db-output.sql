-- Reset dest_customers and dest_orders before running database-output scenarios
-- so each verification run starts from a known state. Keeps the duplicate seed
-- row (CUST-DEST-001 / duplicate@example.test) so transaction-rollback and
-- on-error-skip scenarios can rely on it.
DELETE FROM dest_customers WHERE external_id LIKE 'DBOUT-%';
DELETE FROM dest_orders WHERE external_order_id LIKE 'DBOUT-%';

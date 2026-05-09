-- After db-output-insert: DBOUT-001..003 must be present in dest_customers.
SELECT external_id, email, full_name, segment
FROM dest_customers
WHERE external_id IN ('DBOUT-001','DBOUT-002','DBOUT-003')
ORDER BY external_id;

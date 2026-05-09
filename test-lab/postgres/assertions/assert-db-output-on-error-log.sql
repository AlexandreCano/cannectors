-- After db-output-on-error-log: DBOUT-200 and DBOUT-202 must be present;
-- DBOUT-201 (missing `email`) must be absent because its INSERT violated NOT NULL.
SELECT external_id
FROM dest_customers
WHERE external_id IN ('DBOUT-200','DBOUT-201','DBOUT-202')
ORDER BY external_id;

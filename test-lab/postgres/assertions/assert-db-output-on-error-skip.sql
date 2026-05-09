-- After db-output-on-error-skip (transaction=false, onError=skip):
-- DBOUT-100 and DBOUT-102 must be present, DBOUT-101 must be absent.
SELECT external_id
FROM dest_customers
WHERE external_id IN ('DBOUT-100','DBOUT-101','DBOUT-102')
ORDER BY external_id;

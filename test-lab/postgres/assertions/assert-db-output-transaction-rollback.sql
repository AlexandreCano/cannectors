-- After db-output-transaction-rollback (transaction=true, onError=fail):
-- DBOUT-100 and DBOUT-102 must NOT be present in dest_customers because the
-- whole batch was rolled back when DBOUT-101 collided on email.
SELECT external_id
FROM dest_customers
WHERE external_id IN ('DBOUT-100','DBOUT-101','DBOUT-102')
ORDER BY external_id;

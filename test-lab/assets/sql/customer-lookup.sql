SELECT segment, region, active
FROM customer_reference
WHERE external_id = {{record.customer.id}}
LIMIT 1

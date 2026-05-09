SELECT
    external_order_id,
    customer_external_id,
    product_sku,
    quantity,
    total_amount,
    status
FROM source_orders
WHERE status = 'paid'
ORDER BY id

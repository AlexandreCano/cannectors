insert into products (product_id, sku, price, updated_at)
values ({{record.product_id}}, {{record.sku}}, {{record.price}}, {{record.updated_at}})
on conflict (product_id) do update set
  sku = excluded.sku,
  price = excluded.price,
  updated_at = excluded.updated_at

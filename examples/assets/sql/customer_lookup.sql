select customer_tier, lifecycle_stage, account_owner
from customer_reference
where customer_id = {{record.id}}
limit 1

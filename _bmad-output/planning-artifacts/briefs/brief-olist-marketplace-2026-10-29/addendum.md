---
title: "Addendum: Olist Marketplace Brief"
created: 2026-10-29
updated: 2026-10-29
---

# Addendum: Olist Marketplace Brief

This addendum holds detail for the PRD and architecture phases. None of it is a decision yet.

## Dataset facts (from CSV headers)

| File | Rows (approx.) | Columns |
|---|---|---|
| olist_customers_dataset | 99k | customer_id, customer_unique_id, customer_zip_code_prefix, customer_city, customer_state |
| olist_orders_dataset | 99k | order_id, customer_id, order_status, order_purchase_timestamp, order_approved_at, order_delivered_carrier_date, order_delivered_customer_date, order_estimated_delivery_date |
| olist_order_items_dataset | 113k | order_id, order_item_id, product_id, seller_id, shipping_limit_date, price, freight_value |
| olist_order_payments_dataset | 104k | order_id, payment_sequential, payment_type, payment_installments, payment_value |
| olist_order_reviews_dataset | 105k | (reviews out of scope) |
| olist_products_dataset | 33k | product_id, product_category_name, product_name_lenght, product_description_lenght, product_photos_qty, product_weight_g, product_length_cm, product_height_cm, product_width_cm |
| olist_sellers_dataset | 3k | seller_id, seller_zip_code_prefix, seller_city, seller_state |
| olist_geolocation_dataset | 1M | zip prefix, lat, lng, city, state (many rows per zip prefix) |
| product_category_name_translation | 71 | Portuguese → English category name (file starts with a UTF-8 BOM) |

Things that affect service design:

- `customer_id` is created fresh for every order. The real person is `customer_unique_id`. Customer-service and auth should key on `customer_unique_id`.
- `seller_id` is on order items, not on the order. One order can include items from several sellers, which matters for the seller revenue view.
- One order can have several payments (`payment_sequential`), for example a card plus a voucher.
- `order_estimated_delivery_date` compared with `order_delivered_customer_date` gives the late-delivery rate directly.
- The products columns are misspelled (`lenght`). Rename them in the schema and map the names in the importer.
- There are no product names or prices on products; the price lives on order items. `[ASSUMPTION]` The catalog shows a price derived from historical order items.

## Candidate service map (input for architecture, not decided)

- **catalog-service:** products, categories, sellers
- **customer-service:** customers and demo accounts
- **order-service:** orders, order items, saga orchestration, outbox
- **payment-service:** payments and the simulated gateway
- **analytics-service:** CQRS read models (dashboard, seller revenue), geolocation
- **api-gateway:** routing and JWT validation
- **tools/olist-importer:** CSV import and event replayer

Repo follow-up: delete `services/review-service` from the skeleton and remove it from `go.work`, the Makefile, and the CI path filter.

## Stretch goals

- Stock reservation step in the saga (order → reserve stock → payment), with stock seeded so historical orders cannot fail.

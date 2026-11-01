-- Orders (AD-1, AD-5, AD-15). Money is int64 centavos; dataset ids are uuid.
CREATE TABLE checkouts (
    customer_id     uuid NOT NULL,
    idempotency_key text NOT NULL,
    request_hash    text NOT NULL,
    checkout_id     uuid NOT NULL UNIQUE,
    created_at      timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (customer_id, idempotency_key)
);

CREATE TABLE orders (
    id               uuid PRIMARY KEY,
    customer_id      uuid NOT NULL,
    -- NULL for Historical Orders, which have no Checkout.
    checkout_id      uuid REFERENCES checkouts (checkout_id),
    status           text NOT NULL CHECK (status IN ('created', 'approved', 'processing', 'shipped', 'delivered', 'canceled')),
    purchased_at     timestamptz NOT NULL,
    historical       boolean NOT NULL DEFAULT false,
    voucher_amount   bigint NOT NULL DEFAULT 0 CHECK (voucher_amount >= 0),
    cancel_reason    text,
    voucher_restored boolean NOT NULL DEFAULT false
);

CREATE INDEX orders_customer_idx ON orders (customer_id, purchased_at DESC, id DESC);
CREATE INDEX orders_checkout_idx ON orders (checkout_id);

-- line keeps the item order; a Historical Order may list one Product twice.
CREATE TABLE order_items (
    order_id     uuid NOT NULL REFERENCES orders (id),
    line         int NOT NULL CHECK (line >= 1),
    product_id   uuid NOT NULL,
    seller_id    uuid NOT NULL,
    seller_state char(2) NOT NULL,
    category_en  text NOT NULL,
    price        bigint NOT NULL CHECK (price >= 0),
    freight      bigint NOT NULL CHECK (freight >= 0),
    PRIMARY KEY (order_id, line)
);

CREATE INDEX order_items_seller_idx ON order_items (seller_id);

CREATE TABLE order_status_timeline (
    order_id uuid NOT NULL REFERENCES orders (id),
    status   text NOT NULL CHECK (status IN ('created', 'approved', 'processing', 'shipped', 'delivered', 'canceled')),
    at       timestamptz NOT NULL,
    PRIMARY KEY (order_id, status)
);

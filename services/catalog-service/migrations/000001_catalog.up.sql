-- Reference data imported from the Olist dataset (FR17). Money is int64
-- centavos; Olist ids are uuid.
CREATE TABLE category (
    name_pt text PRIMARY KEY CHECK (name_pt <> ''),
    name_en text NOT NULL CHECK (name_en <> '')
);

-- Products the dataset leaves without a Category belong here.
INSERT INTO category (name_pt, name_en) VALUES ('Uncategorized', 'Uncategorized');

CREATE TABLE seller (
    id    uuid PRIMARY KEY,
    city  text NOT NULL,
    state char(2) NOT NULL
);

CREATE TABLE product (
    id           uuid PRIMARY KEY,
    category     text NOT NULL REFERENCES category (name_pt),
    display_name text NOT NULL,
    price        bigint NOT NULL CHECK (price >= 0),
    freight      bigint NOT NULL CHECK (freight >= 0),
    seller_id    uuid NOT NULL REFERENCES seller (id)
);

CREATE INDEX product_category_idx ON product (category);
CREATE INDEX product_seller_idx ON product (seller_id);

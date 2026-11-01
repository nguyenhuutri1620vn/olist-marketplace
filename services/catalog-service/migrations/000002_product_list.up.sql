-- Product browsing order (Story 1.8): display name ignoring case, then id,
-- under COLLATE "C" so it does not depend on the database locale. Must match
-- productOrder in internal/repository/browse.go to be used.
CREATE INDEX product_list_idx ON product (lower(display_name) COLLATE "C", display_name COLLATE "C", id);
CREATE INDEX product_category_list_idx ON product (category, lower(display_name) COLLATE "C", display_name COLLATE "C", id);

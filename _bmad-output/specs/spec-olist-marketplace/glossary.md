# Glossary

Every artifact downstream of `SPEC.md` uses these terms exactly. No synonyms.

- **Customer** — A person who buys. Dataset Customers are identified by `customer_unique_id`; everyone else registers. Has one Account.
- **Seller** — A shop that lists Products and fulfils Orders. Uses a seeded Account only.
- **Admin** — The single operator Account. Views the Ops Dashboard and creates Vouchers.
- **Account** — Login credentials plus exactly one role: Customer, Seller, or Admin.
- **Product** — An item sold by exactly one Seller, in at most one Category.
- **Category** — A Product grouping, shown by its English name. A Product with no Category shows as "Uncategorized".
- **Checkout** — The Customer's action of buying Products. It produces one Order per Seller.
- **Order** — A purchase made of Order Items, with one Order Status. A new Order always has exactly one Seller; a Historical Order may have several.
- **Order Item** — One Product line in an Order, with a price and a freight value.
- **Order Status** — `created` (awaiting payment), `approved` (fully paid), `processing` (entered automatically after `approved`), `shipped`, `delivered`, or `canceled`.
- **Payment** — Money applied to one Order: at most one Card Payment and at most one Voucher.
- **Card Payment** — A payment through the simulated card gateway. The gateway's decline rate is configurable.
- **Voucher** — A single-use code with a fixed value, created by the Admin. It cannot be used partially.
- **Historical Order** — An Order taken from the Olist CSVs that enters the system only through Replay. It never runs the Order Saga.
- **Order Saga** — The orchestrated sequence that takes a new Order from `created` to `approved` or `canceled`, including Voucher compensation.
- **Seller Revenue** — For `delivered` Orders only: the sum of Order Item price (freight excluded), minus the applied Voucher for new Orders, never below 0 per Order. Historical Orders ignore Vouchers.
- **Import** — A one-time load of Products, Categories, Sellers and Customers from the CSVs.
- **Replay** — Streaming Historical Orders as events, in time order, over a chosen date range and at a chosen speed.

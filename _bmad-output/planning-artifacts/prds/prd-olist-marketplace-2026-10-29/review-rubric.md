# PRD Quality Review — Olist Marketplace (rubric walker, quiet pass for solo stakes)

## Overall verdict
Adequate and decision-ready after fixes. Decisions are explicit, trade-offs logged, NFRs carry product-specific thresholds, counter-metrics present.

## Findings
- **high** Glossary contradiction (§3 Order vs Historical Order) — Order is "from exactly one Seller" but Historical Orders may span Sellers. *Fix:* scope the one-Seller rule to new Orders.
- **high** Missing FR for Voucher creation (§3 Admin "manages Vouchers", §4.6 FR-16) — no FR lets Admin create Vouchers, yet FR-2/FR-3 depend on them. *Fix:* add FR-19.
- **medium** NFR-1 calls `processing` a final Order Status. *Fix:* say the Order leaves `created`.
- **medium** Glossary uses undefined "Voucher Payment" (§3 Payment). *Fix:* reword to Voucher.
- **medium** Customer defined by `customer_unique_id`, but registered Customers have none (§3). *Fix:* scope to dataset Customers.
- **low** "estimated delivery dates" listed under FR-2 Out of Scope; belongs with FR-7.
- **low** Assumption tags remain after user confirmation (§4.2, §4.4, §5, §10). *Fix:* remove tags, mark index confirmed.

## Substance over theater — pass
No persona or NFR theater; UJs intentionally omitted for a solo technical PRD.

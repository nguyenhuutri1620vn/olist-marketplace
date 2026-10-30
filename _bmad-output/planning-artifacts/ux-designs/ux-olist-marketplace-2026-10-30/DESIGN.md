---
name: Olist Marketplace
description: Simple demo web UI for the Olist Marketplace learning project. shadcn/ui on a React 19 + Vite SPA with Tailwind; this file specifies only the small delta on top of shadcn defaults.
status: final
updated: 2026-10-30
sources:
  - ../../prds/prd-olist-marketplace-2026-10-29/prd.md
  - ../../architecture/architecture-olist-marketplace-2026-10-30/ARCHITECTURE-SPINE.md
colors:
  # [ASSUMPTION] Everything not listed inherits shadcn defaults (neutral base):
  # background, foreground, muted, border, input, ring, card, popover, primary, destructive.
  # Only Order Status badge colors are added, because status is the one thing every screen shows.
  status-created: '#64748B'     # slate-500
  status-approved: '#2563EB'    # blue-600
  status-processing: '#D97706'  # amber-600
  status-shipped: '#7C3AED'     # violet-600
  status-delivered: '#16A34A'   # green-600
  status-canceled: '#DC2626'    # red-600
  status-foreground: '#FFFFFF'
typography:
  # shadcn defaults (system sans stack) inherited. One numeric role added for money and counts.
  numeric:
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, monospace'
    fontWeight: '500'
rounded:
  # shadcn defaults inherited; no overrides.
spacing:
  # Tailwind 4-based scale inherited; one layout token.
  content-max: 1152px
components:
  status-badge:
    foreground: '{colors.status-foreground}'
    radius: 9999px
    note: 'background = colors.status-<order status>'
  money:
    fontFamily: '{typography.numeric.fontFamily}'
    align: right
---

## Brand & Style

This is a demo console for a backend learning project, not a shop people will use. PRD counter-metric SM-C1 says UI polish is not a goal, so the visual layer is deliberately plain. It uses shadcn/ui defaults as-is, with no brand color, no custom fonts, no imagery and no illustrations. The UI exists to make backend behavior visible: Order Status changes, saga outcomes, revenue numbers and replay counts. [ASSUMPTION]

## Colors

- **Neutral shadcn palette** for all chrome, surfaces, buttons and text. The default `primary` (near-black) stays.
- **Order Status colors** (`{colors.status-created}` … `{colors.status-canceled}`) are the only added colors. They appear only on the status badge and on the matching row of the Ops Dashboard "Orders by status" table. They are never used for buttons or decoration.
- `destructive` (shadcn) is used for error toasts and payment-declined messages.

## Typography

shadcn's default sans ramp is used for everything. Money amounts and counts use `{typography.numeric}`, right-aligned, so columns of numbers line up in tables. Amounts are shown as `R$ 1.234,56`, because the Olist data is in BRL. [ASSUMPTION]

## Layout & Spacing

There is a top navigation bar and a single content column capped at `{spacing.content-max}`. There is no sidebar. Pages are made of a page title, an optional filter row, then a table or a form. Desktop is the only target; narrow screens just scroll.

## Components

These shadcn components are used unchanged: `Button`, `Input`, `Label`, `Select`, `Table`, `Card`, `Checkbox`, `Badge`, `Dialog`, `Toast` (Sonner), `Tabs`, `Skeleton`, `DropdownMenu`. The `Calendar`/date inputs are native `<input type="date">` to keep it simple.

Added:

- **Status badge** is a pill (`Badge`) filled with the colour for its Order Status, with white text showing the status name exactly as the dataset spells it (`created`, `approved`, `processing`, `shipped`, `delivered`, `canceled`).
- **Money** is a right-aligned value set in the numeric style and formatted as BRL.
- **Bar cell** is a thin horizontal bar in the `muted-foreground` colour, drawn behind a number in dashboard tables. Its width is the row's share of the column maximum. There is no chart library.

## Do's and Don'ts

| Do | Don't |
|---|---|
| Use shadcn components unchanged | Add a brand theme, gradients, or a custom font |
| Show status as a coloured badge with the exact dataset word | Rename statuses ("Paid", "In transit") |
| Right-align money and counts | Mix currency formats |
| Tables and forms | Product images, carousels, hero banners |

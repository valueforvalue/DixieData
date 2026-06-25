# ADR-0003 Token reference

The canonical token vocabulary locked by ADR-0003. Every component in
`internal/templates/components/*.templ` (Phase 1 of issue #74) and every
existing template refactor must reference these names. New tokens require
an ADR amendment.

## Color

| Token              | Value                              | Use                                  |
| ------------------ | ---------------------------------- | ------------------------------------ |
| `gold`             | `#a88a46`                          | Brand accent, headings, gold rule    |
| `sepia-500`        | `#8d7440`                          | Borders, dividers, button focus ring |
| `sepia-300`        | `#cfb77a`                          | Hover / focus accent                 |
| `parchment`        | `rgba(246,241,228,0.98)`           | Modal surface, raised panels         |
| `parchment-soft`   | `rgba(246,241,228,0.72)`           | Inline surface (settings cards)      |
| `ink`              | `#22303d`                          | Body text                            |
| `ink-muted`        | `rgba(34,45,57,0.7)`               | Secondary text, captions             |
| `ink-faint`        | `rgba(34,45,57,0.025)`             | Repeating texture overlay            |
| `bg-amber-50`      | `rgba(245,241,230,0.97)`           | Highlighted result row surface       |
| `bg-slate-200`     | `rgba(223,228,234,0.92)`           | `.card` resting surface              |
| `review-red`       | `#6f2c26`                          | "Needs Review" pill                  |
| `review-red-tint`  | `rgba(111,44,38,0.12)`             | "Needs Review" pill background       |
| `success-green`    | `#29522d`                          | Settings success text                |
| `success-green-bg` | `rgba(242,252,244,0.95)`           | Settings success surface             |
| `error-red`        | `#7a2d2d`                          | Settings error text                  |
| `error-red-bg`     | `rgba(255,245,245,0.95)`           | Settings error surface               |

## Radius

| Token         | Value     | Use                              |
| ------------- | --------- | -------------------------------- |
| `surface`     | `1.7rem`  | Top shell, cards, dock           |
| `surface-sm`  | `1.2rem`  | Inner nested panels              |
| `dialog`      | `2rem`    | Modal dialogs                    |
| `field`       | `0.65rem` | Form fields, checkboxes          |

## Shadow

| Token     | Value                                       | Use                |
| --------- | ------------------------------------------- | ------------------ |
| `card`    | `0 16px 32px rgba(23,33,43,0.16)`           | Resting card       |
| `card-lg` | `0 20px 40px rgba(21,29,38,0.2)`            | Top shell, dock    |
| `modal`   | `0 24px 60px rgba(15,23,42,0.35)`           | Modal dialog       |
| `modal-lg`| `0 24px 44px rgba(23,33,43,0.28)`           | Floating nav panel |
| `pop`     | `0 0 30px rgba(197,171,104,0.16)`           | Highlighted result |

## Motion

| Token   | Value            | Use                       |
| ------- | ---------------- | ------------------------- |
| `fast`  | `120ms ease-out` | Hover, focus, button feedback |
| `med`   | `240ms ease-out` | Toast slide-in, panel open    |
| `slow`  | `400ms ease-out` | Page transitions (Phase 5)    |

## Typography

Tailwind defaults apply. No custom font family, scale, or letter-spacing
tokens yet. If Phase 5 introduces a serif accent, lock it here.

## Spacing rhythm

Tailwind defaults apply (4px scale). Phase 2 introduces `data-density`
which doubles or halves the gap utility values at the root element via
CSS variable overrides — no new tokens required.
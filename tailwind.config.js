module.exports = {
  content: [
    "./frontend/**/*.{html,js}",
    "./internal/**/*.{templ,go}",
  ],
  theme: {
    extend: {
      // Design tokens — see docs/adr/0003-design-system-tokens.md
      // Locked vocabulary for issue #74 Phase 1 component primitives.
      // Hex literal migrations are gated per-component-class in #74 PRs
      // (card, field-input, primary-button, ...). This PR just adds the
      // names so component work in subsequent PRs can reference them.
      colors: {
        gold: "#a88a46",
        "sepia-500": "#8d7440",
        parchment: "rgba(246,241,228,0.98)",
        "parchment-soft": "rgba(246,241,228,0.72)",
        ink: "#22303d",
        "ink-muted": "rgba(34,45,57,0.7)",
        "ink-faint": "rgba(34,45,57,0.025)",
        "bg-amber-50": "rgba(245,241,230,0.97)",
        "bg-slate-200": "rgba(223,228,234,0.92)",
        "bg-sepia-top": "#d7d2c9",
        "bg-sepia-mid": "#c9c2b5",
        "bg-sepia-bottom": "#b9b1a3",
        "review-red": "#6f2c26",
        "review-red-tint": "rgba(111,44,38,0.12)",
        "success-green": "#29522d",
        "success-green-bg": "rgba(242,252,244,0.95)",
        "error-red": "#7a2d2d",
        "error-red-bg": "rgba(255,245,245,0.95)",
        // Research / relationship accent palette — used on the
        // Camaraderie, Conflict Ledger, Research Log / Pack /
        // Collections, Service Timeline, and the matching side-cards
        // on Soldier Detail. Hex literals map to Tailwind's default
        // blue-50 / 100 / 200 / 600 / 700. Tokenized in issue #173
        // so the intent ("research content") is named in code.
        "research-bg": "#eff6ff",
        "research-border-soft": "#dbeafe",
        "research-border": "#bfdbfe",
        "research-accent": "#2563eb",
        "research-text": "#1d4ed8",
      },
      borderRadius: {
        surface: "1.7rem",
        "surface-sm": "1.2rem",
        dialog: "2rem",
        field: "0.65rem",
      },
      boxShadow: {
        card: "0 16px 32px rgba(23,33,43,0.16)",
        "card-lg": "0 20px 40px rgba(21,29,38,0.2)",
        modal: "0 24px 60px rgba(15,23,42,0.35)",
        "modal-lg": "0 24px 44px rgba(23,33,43,0.28)",
        pop: "0 0 30px rgba(197,171,104,0.16)",
      },
      transitionDuration: {
        fast: "120ms",
        med: "240ms",
        slow: "400ms",
      },
    },
  },
  plugins: [],
};
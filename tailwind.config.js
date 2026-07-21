module.exports = (function() {
  // Read palette from config.default.json so there is one source
  // of truth shared with the Typst templates (#637). The file is
  // at the repo root; during the CSS build, Node resolves it
  // relative to this config file.
  var palette = {};
  try {
    var cfg = require('./config.default.json');
    palette = cfg.theme && cfg.theme.palette || {};
  } catch (_) {
    // Fallback: config file missing (pre-build state, CI, etc.).
    // Use hard-coded defaults that match config.go Defaults().
    palette = {
      accent: "#8d7440",
      accent_strong: "#a88a46",
      text_primary: "#22303d",
      text_secondary: "#445260",
      text_muted: "#71808e",
      link: "#4A90E2",
      danger: "#54211d",
      divider: "#8d7440",
      panel_fill: "#fff8e7",
    };
  }

  return {
    content: [
      "./frontend/**/*.{html,js}",
      "./internal/**/*.{templ,go}",
    ],
    theme: {
      extend: {
        colors: {
          // Palette-driven tokens (from config.default.json theme.palette):
          gold: palette.accent_strong,
          "gold-light": "#c5ab68",
          "gold-deep": "#a5853f",
          "gold-glow": "#eddca6",
          "sepia-500": palette.accent,
          "sepia-300": "#cfb77a",
          parchment: "rgba(246,241,228,0.98)",
          "parchment-soft": "rgba(246,241,228,0.72)",
          ink: palette.text_primary,
          "ink-deep": "#1f2b38",
          "ink-mid": "#324253",
          "ink-muted": "rgba(34,45,57,0.7)",
          "ink-faint": "rgba(34,45,57,0.025)",
          "bg-amber-50": "rgba(245,241,230,0.97)",
          "bg-slate-200": "rgba(223,228,234,0.92)",
          "bg-sepia-top": "#d7d2c9",
          "bg-sepia-mid": "#c9c2b5",
          "bg-sepia-bottom": "#b9b1a3",
          "review-red": "#6f2c26",
          "review-red-deep": palette.danger,
          "review-red-tint": "rgba(111,44,38,0.12)",
          "success-green": "#29522d",
          "success-green-bg": "rgba(242,252,244,0.95)",
          "error-red": "#7a2d2d",
          "error-red-bg": "rgba(255,245,245,0.95)",
          warning: "#d97706",
          warning_bg: "rgba(255,251,235,0.99)",
          info: "#2563eb",
          "info-bg": "rgba(239,246,255,0.99)",
          "research-bg": "#eff6ff",
          "research-border-soft": "#dbeafe",
          "research-border": "#bfdbfe",
          "research-accent": "#2563eb",
          "research-text": "#1d4ed8",
          "shadow-ink": "#17212b",
          "shadow-deep": "#0f172a",
          "accent-coral": "#cd565a",
          "gold-sand": "#f2d185",
          "sepia-700": "#7c6c52",
          "surface-warm": "#d8d8d7",
          "ink-slate": "#666870",
          "gold-tan": "#a89c8b",
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
})();
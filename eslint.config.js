// ESLint flat config (v9). Applies the DixieData local rules
// to every JS file under frontend/.
//
// `frontend/wailsjs/`, `frontend/htmx.min.js`, and
// `frontend/debug.test.mjs` are excluded (Wails-generated
// bindings, vendored htmx, and the hostile-input test that
// uses `assert.doesNotThrow(() => {})` — the rule's regex
// does not match the `assert.doesNotThrow` shape, but
// exclusion is safer for future-proofing).
//
// See docs/adr/0010-lint-enforcement.md and issue #438.

"use strict";

const js = require("@eslint/js");
const globals = require("globals");
const dixiePlugin = require("./frontend/eslint-plugin-dixie/index.js");

module.exports = [
  {
    ignores: [
      "frontend/wailsjs/**",
      "frontend/htmx.min.js",
      "frontend/debug.test.mjs",
      // The fixture files are inputs to the no-bare-catch rule,
      // not code that should be linted.
      "frontend/eslint-plugin-dixie/fixtures/**",
    ],
  },
  js.configs.recommended,
  {
    files: ["frontend/**/*.js"],
    languageOptions: {
      ecmaVersion: 2022,
      sourceType: "script",
      globals: {
        ...globals.browser,
        ...globals.node,
      },
    },
    plugins: {
      dixie: dixiePlugin,
    },
    rules: {
      "dixie/no-bare-catch": "error",
      // Project-wide relaxation for the recommended preset;
      // existing files predate this config and use patterns
      // that the recommended preset flags but are not the
      // swallowed-error class this config targets.
      "no-unused-vars": "off",
      "no-undef": "off",
      "no-empty": "off",
      "no-useless-escape": "off",
    },
  },
];

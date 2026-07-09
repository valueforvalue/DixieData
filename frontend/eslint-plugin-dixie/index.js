// eslint-plugin-dixie — DixieData's local ESLint rules.
//
// no-bare-catch: flags `.catch(() => {})` and
// `.catch((e) => {})` without a `// intentional: <reason>`
// comment on the immediately-preceding source line. The
// "intentional" keyword is the shared marker with the
// `audit/smoke_swallowed_errors.mjs` regression probe (the
// probe's section 5 walks `frontend/` for the same pattern).
//
// Bail-out: the `// intentional` keyword must appear in a
// comment on the line directly above the catch. This matches
// the convention locked in by issue #436 (the 6 debug logger
// sites in `frontend/debug.js`).
//
// See docs/adr/0010-lint-enforcement.md and issue #438.

"use strict";

const INTENTIONAL_KEYWORD = "intentional";

const meta = {
  type: "problem",
  docs: {
    description:
      "Disallow bare `.catch(() => {})` or `.catch((e) => {})` without a `// intentional` comment marker (see docs/agents/error-handling.md, issue #438).",
    recommended: true,
  },
  schema: [],
  messages: {
    bareCatch:
      "Bare `.catch(…)` discards the error. Add `console.warn(context, err)` (or use showToast) per error-handling.md, or add a `// intentional: <reason>` comment on the line above if this site is genuinely a never-throw component.",
  },
};

function create(context) {
  const sourceCode = context.sourceCode || context.getSourceCode();

  return {
    CallExpression(node) {
      // Match: <expr>.catch(<arrow>)
      if (
        !node.callee ||
        node.callee.type !== "MemberExpression" ||
        !node.callee.property ||
        node.callee.property.name !== "catch"
      ) {
        return;
      }
      // Must have exactly one argument.
      if (!node.arguments || node.arguments.length !== 1) {
        return;
      }
      const arg = node.arguments[0];
      // Must be an arrow function expression with a block body.
      if (
        arg.type !== "ArrowFunctionExpression" ||
        !arg.body ||
        arg.body.type !== "BlockStatement"
      ) {
        return;
      }
      // Body must be empty (no statements, no params usage).
      if (arg.body.body.length > 0) {
        return;
      }
      // Bail-out: the line above must contain `// intentional`.
      const catchLine = node.loc.start.line;
      if (catchLine > 1) {
        const above = sourceCode.lines[catchLine - 2] || "";
        if (above.indexOf(INTENTIONAL_KEYWORD) !== -1) {
          return;
        }
      }
      context.report({ node, messageId: "bareCatch" });
    },
  };
}

module.exports = {
  rules: {
    "no-bare-catch": { meta, create },
  },
};

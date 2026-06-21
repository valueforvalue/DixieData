/**
 * DixieData workflows.
 *
 * Three workflows, all sharing the same artifact directory tree at
 * `.rpiv/artifacts/<bucket>/`:
 *
 *   plan-build       — discover → research → explore → plan →
 *                      lock-requirements → frontend-design →
 *                      ui-ux-audit → commit
 *
 *   design-it-twice  — fanout 3 design proposals, panel-review with
 *                      3 judges (correctness, fit, actionability),
 *                      pick winner, write doc, commit
 *
 *   ship-issue       — implement one issue, code-review, revise or
 *                      commit-and-close (gated on blockers_count)
 *
 * Default: `plan-build`. Run any of them via `/wf <name> "<input>"`.
 */

import {
  defineWorkflow,
  produces,
  acts,
  directoryPathCollector,
  gitCommitOutcome,
  fanout,
  panel,
  judge,
  majority,
  verify,
  gate,
  gt,
  eq,
  typeboxSchema,
  jsonBodyParser,
} from "@juicesharp/rpiv-workflow";
import type { PanelVerdict } from "@juicesharp/rpiv-workflow";
import { Type } from "@sinclair/typebox";

/* ============================================================
 * Shared artifact outcomes
 * ============================================================ */

const FRD_OUTCOME = {
  name: "frd",
  collector: directoryPathCollector({
    dir: ".rpiv/artifacts/discover",
    ext: "md",
  }),
};

const RESEARCH_OUTCOME = {
  name: "research",
  collector: directoryPathCollector({
    dir: ".rpiv/artifacts/research",
    ext: "md",
  }),
};

const SOLUTIONS_OUTCOME = {
  name: "solutions",
  collector: directoryPathCollector({
    dir: ".rpiv/artifacts/solutions",
    ext: "md",
  }),
};

const PLAN_OUTCOME = {
  name: "plan",
  collector: directoryPathCollector({
    dir: ".rpiv/artifacts/plans",
    ext: "md",
  }),
};

const AUDIT_OUTCOME = {
  name: "audit",
  collector: directoryPathCollector({
    dir: ".rpiv/artifacts/audit",
    ext: "md",
  }),
};

const DESIGNS_OUTCOME = {
  name: "designs",
  collector: directoryPathCollector({
    dir: ".rpiv/artifacts/designs",
    ext: "md",
  }),
};

const IMPL_OUTCOME = {
  name: "impl",
  collector: directoryPathCollector({
    dir: ".rpiv/artifacts/impl",
    ext: "md",
  }),
};

const REVIEW_OUTCOME = {
  name: "review",
  collector: directoryPathCollector({
    dir: ".rpiv/artifacts/reviews",
    ext: "json",
  }),
  parser: jsonBodyParser,
};

const REVIEW_SCHEMA = typeboxSchema(
  Type.Object(
    { blockers_count: Type.Integer({ minimum: 0 }) },
    { additionalProperties: true },
  ),
);

/* ============================================================
 * plan-build
 * ============================================================ */

const discover = produces({ outcome: FRD_OUTCOME });

const researchStage = produces({
  outcome: RESEARCH_OUTCOME,
  reads: ["frd"],
});

const explore = produces({
  outcome: SOLUTIONS_OUTCOME,
  reads: ["research"],
});

const planStage = produces({
  outcome: PLAN_OUTCOME,
  reads: ["solutions"],
});

const lockRequirements = acts({
  skill: "lock-requirements",
  sessionPolicy: "continue",
});

const frontendDesign = acts({
  skill: "frontend-design",
  sessionPolicy: "continue",
});

const uiUxAudit = produces({
  outcome: AUDIT_OUTCOME,
  reads: ["plan"],
});

const planCommit = acts({ outcome: gitCommitOutcome });

export const planBuild = defineWorkflow({
  name: "plan-build",
  description:
    "Discover → research → explore → plan → lock-requirements → frontend-design → ui-ux-audit → commit",
  start: "discover",
  stages: {
    discover,
    research: researchStage,
    explore,
    plan: planStage,
    "lock-requirements": lockRequirements,
    "frontend-design": frontendDesign,
    "ui-ux-audit": uiUxAudit,
    commit: planCommit,
  },

  edges: {
    discover: "research",
    research: "explore",
    explore: "plan",
    plan: "lock-requirements",
    "lock-requirements": "frontend-design",
    "frontend-design": "ui-ux-audit",
    "ui-ux-audit": "commit",
    commit: "stop",
  },
});

/* ============================================================
 * design-it-twice
 *
 * 3 parallel design proposals (alpha/delta/gamma), then a 3-judge
 * panel (correctness/fit/actionability) folds to one winner.
 * ============================================================ */

const DESIGN_VERDICT_OUTCOMES = {
  correctness: {
    name: "v-correctness",
    collector: directoryPathCollector({
      dir: ".rpiv/artifacts/reviews",
      ext: "json",
    }),
  },
  fit: {
    name: "v-fit",
    collector: directoryPathCollector({
      dir: ".rpiv/artifacts/reviews",
      ext: "json",
    }),
  },
  actionability: {
    name: "v-actionability",
    collector: directoryPathCollector({
      dir: ".rpiv/artifacts/reviews",
      ext: "json",
    }),
  },
};

const DESIGN_FANOUT = fanout({
  source: "designs",
  unit: { by: "manual-list", pattern: "alpha-delta-gamma" },
  units: ({ state }) => {
    const brief = state.originalInput;
    return [
      {
        id: "alpha",
        label: "design alpha",
        prompt: `Generate design proposal ALPHA for: ${brief}. Constraints: minimal public surface, deep-module pattern, no new deps.`,
      },
      {
        id: "delta",
        label: "design delta",
        prompt: `Generate design proposal DELTA for: ${brief}. Constraints: function-first, generous params, leans on existing helpers.`,
      },
      {
        id: "gamma",
        label: "design gamma",
        prompt: `Generate design proposal GAMMA for: ${brief}. Constraints: facade + worker split, explicit data model, testable in isolation.`,
      },
    ];
  },
});

const fanoutDesign = produces({
  outcome: DESIGNS_OUTCOME,
  loop: DESIGN_FANOUT,
});

const REVIEW_PANEL = panel({
  members: [
    judge({
      skill: "review-correctness",
      outcome: DESIGN_VERDICT_OUTCOMES.correctness,
    }),
    judge({
      skill: "review-fit",
      outcome: DESIGN_VERDICT_OUTCOMES.fit,
    }),
    judge({
      skill: "review-actionability",
      outcome: DESIGN_VERDICT_OUTCOMES.actionability,
    }),
  ],
  fold: majority((v) => (v.data as { ok?: boolean }).ok === true),
});

const panelReview = produces({
  outcome: {
    name: "design-winner",
    collector: directoryPathCollector({
      dir: ".rpiv/artifacts/designs",
      ext: "md",
    }),
  },
  verify: verify({
    judge: REVIEW_PANEL,
    done: (v) => (v.data as PanelVerdict).pass,
  }),
  reads: ["designs"],
});

const writeup = produces({
  outcome: {
    name: "design-doc",
    collector: directoryPathCollector({
      dir: ".rpiv/artifacts/designs",
      ext: "md",
    }),
  },
  reads: ["design-winner"],
});

const designCommit = acts({ outcome: gitCommitOutcome });

export const designItTwice = defineWorkflow({
  name: "design-it-twice",
  description:
    "Fanout 3 design proposals, panel-review them with 3 judges, pick the winner, write a design doc, commit.",
  start: "fanout-design",
  stages: {
    "fanout-design": fanoutDesign,
    "panel-review": panelReview,
    writeup,
    commit: designCommit,
  },
  edges: {
    "fanout-design": "panel-review",
    "panel-review": "writeup",
    writeup: "commit",
    commit: "stop",
  },
});

/* ============================================================
 * ship-issue
 *
 * implement → review → (revise → review)* | commit
 * gated on blockers_count.
 * ============================================================ */

const implementStage = produces({ outcome: IMPL_OUTCOME });

const reviewStage = produces({
  outcome: REVIEW_OUTCOME,
  outputSchema: REVIEW_SCHEMA,
  reads: ["impl"],
});

const revise = produces({
  outcome: {
    name: "revise",
    collector: directoryPathCollector({
      dir: ".rpiv/artifacts/impl",
      ext: "md",
    }),
  },
  reads: ["impl", "review"],
});

const shipCommit = acts({ outcome: gitCommitOutcome });

export const shipIssue = defineWorkflow({
  name: "ship-issue",
  description:
    "Implement one issue, code-review it, revise if blockers, commit-and-close when clean.",
  start: "implement",
  stages: {
    implement: implementStage,
    review: reviewStage,
    revise,
    commit: shipCommit,
  },
  edges: {
    implement: "review",
    review: gate(
      "blockers_count",
      {
        revise: gt(0),
        commit: eq(0),
      },
      "revise", // otherwise
    ),
    revise: "review", // re-review after revise; loop until blockers = 0
    commit: "stop",
  },
});

/* ============================================================
 * Default export (envelope form)
 * ============================================================ */

export default {
  workflows: [planBuild, designItTwice, shipIssue],
  default: "plan-build",
};

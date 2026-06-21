const path = require("path");
const Module = require("module");
const origResolve = Module._resolveFilename;
Module._resolveFilename = function (request, parent, ...args) {
  // Subpath imports in @sinclair/typebox live in build/cjs/<subpath>/index.js
  // but the subpath package.json is misnamed for direct resolve in some loaders.
  if (request === "typebox/value" && parent && parent.filename && parent.filename.includes("typebox")) {
    request = "@sinclair/typebox/value";
  }
  return origResolve.call(this, request, parent, ...args);
};

const jiti = require("C:/Users/value/.pi/agent/npm/node_modules/jiti")(__filename, {
  interopDefault: true,
  esmResolve: true,
});

const configPath = path.resolve(".rpiv/workflows/config.ts");
const config = jiti(configPath);
const { validateWorkflow } = jiti("@juicesharp/rpiv-workflow");

const wfs = (config.default && config.default.workflows) || [config.planBuild, config.designItTwice, config.shipIssue].filter(Boolean);
const def = config.default;

console.log("=".repeat(72));
console.log("rpiv-workflow dry-run: " + wfs.length + " workflow(s)");
console.log("Default: " + (def && def.default ? def.default : "(none)"));
console.log("=".repeat(72));

let totalIssues = 0;
for (const wf of wfs) {
  console.log("\n--- " + wf.name + " ---");
  console.log("Description: " + (wf.description || "(none)"));
  console.log("Start:       " + wf.start);
  console.log("Stages:      " + Object.keys(wf.stages).join(", "));
  console.log("Edges:       " + Object.keys(wf.edges).length);

  let issues;
  try {
    issues = validateWorkflow(wf);
  } catch (e) {
    console.log("VALIDATION THREW: " + e.message);
    totalIssues++;
    continue;
  }

  if (!issues || issues.length === 0) {
    console.log("VALIDATION:  OK");
  } else {
    totalIssues += issues.length;
    console.log("VALIDATION:  " + issues.length + " issue(s)");
    for (const i of issues) {
      const code = (i && i.code) || "?";
      const msg = (i && i.message) || JSON.stringify(i);
      console.log("  - " + code + ": " + msg);
      if (i && i.path) console.log("      path: " + i.path);
    }
  }
}

console.log("\n" + "=".repeat(72));
console.log(totalIssues === 0 ? "ALL WORKFLOWS VALID" : "TOTAL ISSUES: " + totalIssues);
console.log("=".repeat(72));
process.exit(totalIssues === 0 ? 0 : 1);

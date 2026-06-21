const path = require("path");
const jiti = require("C:/Users/value/.pi/agent/npm/node_modules/jiti")(__filename, {
  interopDefault: true,
  esmResolve: true,
});

const config = jiti(path.resolve(".rpiv/workflows/config.ts"));
const wfs = (config.default && config.default.workflows) || [config.planBuild, config.designItTwice, config.shipIssue].filter(Boolean);

for (const wf of wfs) {
  console.log("--- " + wf.name + " ---");
  const stages = Object.keys(wf.stages);
  const seen = new Set();
  const stack = [wf.start];
  const path_ = [];
  while (stack.length > 0) {
    const s = stack.pop();
    if (seen.has(s)) continue;
    seen.add(s);
    path_.push(s);
    const edge = wf.edges[s];
    if (edge === undefined) continue;
    if (edge === "stop") continue;
    if (typeof edge === "string") { stack.push(edge); continue; }
    if (typeof edge === "function") {
      // EdgeFn: collect targets from the .targets property if attached.
      const t = edge.targets || [];
      for (const x of t) stack.push(x);
      continue;
    }
  }
  console.log("Reached: " + path_.join(" -> "));
  const unreachable = stages.filter(s => !seen.has(s) && s !== wf.start);
  const notReached = stages.filter(s => !seen.has(s));
  if (notReached.length > 0) {
    console.log("UNREACHED STAGES: " + notReached.join(", "));
  } else {
    console.log("All stages reachable from '" + wf.start + "'.");
  }
  // Check every stage has an edge entry
  const missingEdge = stages.filter(s => wf.edges[s] === undefined);
  if (missingEdge.length > 0) {
    console.log("MISSING EDGE: " + missingEdge.join(", "));
  }
  // Check terminal sentinel
  const hasStop = Object.values(wf.edges).some(e => e === "stop");
  console.log("Reaches 'stop': " + hasStop);
  console.log("");
}

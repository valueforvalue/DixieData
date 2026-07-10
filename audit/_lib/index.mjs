// audit/_lib/index.mjs — barrel re-export for the audit harness
// helpers. Probes that need more than one helper can `import {
// helperA, helperB } from './_lib/index.mjs'` instead of importing
// each module separately. Single-file helpers stay preferred for
// probes that only need one thing (keeps the import list short
// and the failure mode obvious).

export { registerCleanup, runWithCleanup } from './cleanup.mjs';
export { setFileChooserFixture } from './filechooser.mjs';
export { resolveWebTestBin, resolveWebBin, resolveProbeDataDir } from './paths.mjs';
// Fixture for eslint-plugin-dixie no-bare-catch rule.
// These are the EXPECTED violations when lint runs.
fetch("/a").catch(() => {});

fetch("/b").catch((e) => {});

fetch("/c").then(() => {}).catch(() => {});

// Good: line above has // intentional keyword
fetch("/d").catch(() => {});

// Good: non-empty catch body
fetch("/e").catch((err) => { console.warn(err); });

// Good: argument is a function reference (not an arrow)
fetch("/f").catch(noop);

// Good: arrow with expression body (not block body)
fetch("/g").catch((e) => console.log(e));

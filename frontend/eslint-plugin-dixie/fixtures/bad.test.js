// GOOD-only fixture — should produce ZERO violations.
// Used to verify the rule accepts all the legitimate forms.
fetch("/a").catch((err) => {
  console.warn("a failed", err);
});

// intentional: never-throw logger
fetch("/b").catch(() => {});

// intentional: dedup catch swallows benign promise rejection
fetch("/c").catch((e) => {});

// Edge: chain with non-empty intermediate
fetch("/d").then((r) => r.json()).catch((err) => {
  showToast("Could not load data", "error");
  console.warn(err);
});

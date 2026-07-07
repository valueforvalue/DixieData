package cookies

import "crypto/rand"

// randReader is the package-level random source. Indirected
// through a var so tests can substitute a deterministic source
// in the future (currently no test does).
var randReader = rand.Reader
package cdb

import "sync"

// testMetrics returns the Metrics shared by the tests: newMetrics registers its
// collectors in the default prometheus registry, so a second call panics.
var testMetrics = sync.OnceValue(newMetrics)

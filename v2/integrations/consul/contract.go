package consul

import "github.com/dreamsxin/go-kit/v2/sd"

// Compile-time contract assertions: this provider is consumed through the
// core service-discovery contracts, so a drift between the two sides must
// fail here instead of inside an application build.
var (
	_ sd.Instancer = (*Instancer)(nil)
	_ sd.Registrar = (*Registrar)(nil)
)

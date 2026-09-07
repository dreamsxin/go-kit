package kit_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"

	"github.com/dreamsxin/go-kit/v2/kit"
	kitgrpc "github.com/dreamsxin/go-kit/v2/kit/grpc"
)

// Transport parity.
//
// Thirteen milestones went into the HTTP surface, and the gRPC one came along for
// some of it. The gap was not visible anywhere: nothing failed when a lifecycle
// contract was added on one side only, so Host.Drain silently skipped the gRPC
// component for two releases.
//
// This file is that gate, in two halves. The assertions below fail to compile when a
// contract is dropped from either transport. The test fails when kit grows an
// interface nobody has classified — which is the moment to decide whether the second
// transport owes an implementation, rather than discovering it later.

// Contracts both serving components must satisfy. Adding a line here without
// implementing it is a compile error, which is the point.
var (
	_ kit.Lifecycle = (*kit.HTTP)(nil)
	_ kit.Lifecycle = (*kitgrpc.Component)(nil)

	_ kit.NamedLifecycle = (*kit.HTTP)(nil)

	_ kit.Draining = (*kit.HTTP)(nil)
	_ kit.Draining = (*kitgrpc.Component)(nil)

	_ kit.ReadinessSink = (*kit.HTTP)(nil)
	_ kit.ReadinessSink = (*kitgrpc.Component)(nil)
)

// transportContracts classifies every exported interface in package kit: true when
// both serving components must implement it, false when it belongs to somebody else.
// The reason is recorded because "false" is a decision, not an omission.
var transportContracts = map[string]struct {
	bothTransports bool
	reason         string
}{
	"Lifecycle":         {true, "a component a Host can start and stop"},
	"Draining":          {true, "the announcement has to reach every server, or a stream never learns"},
	"ReadinessSink":     {true, "each transport serves readiness in its own protocol: /readyz, grpc.health.v1"},
	"NamedLifecycle":    {false, "diagnostics, not a contract: kit/grpc.Component is named by its type in Host output"},
	"ReadinessProvider": {false, "implemented by application components with warm-up, not by transports"},
	"CertificateSource": {false, "implemented by the deployment, not by a transport: it answers where a certificate comes from, and only the HTTP listener terminates TLS here"},
}

func TestEveryKitContractIsClassifiedForBothTransports(t *testing.T) {
	fileSet := token.NewFileSet()
	packages, err := parser.ParseDir(fileSet, ".", nil, 0)
	if err != nil {
		t.Fatalf("parse the kit package: %v", err)
	}

	found := map[string]bool{}
	for name, pkg := range packages {
		if name != "kit" {
			continue
		}
		ast.Inspect(pkg, func(node ast.Node) bool {
			spec, ok := node.(*ast.TypeSpec)
			if !ok || !spec.Name.IsExported() {
				return true
			}
			if _, isInterface := spec.Type.(*ast.InterfaceType); !isInterface {
				return true
			}
			found[spec.Name.Name] = true
			return true
		})
	}
	if len(found) == 0 {
		t.Fatal("found no exported interfaces in package kit; the scan is broken, not the package")
	}

	for name := range found {
		if _, classified := transportContracts[name]; !classified {
			t.Errorf("kit.%s is an exported interface nothing has classified.\n\n"+
				"Decide whether both serving components owe an implementation and record it in "+
				"transportContracts, with the reason. An unclassified contract is how one transport "+
				"quietly gains something the other lacks.", name)
		}
	}
	for name := range transportContracts {
		if !found[name] {
			t.Errorf("transportContracts lists kit.%s, which package kit no longer declares; remove the entry", name)
		}
	}
}

package ginadapter_test

import (
	"testing"

	ginadapter "github.com/ashrafAli23/nestgo-gin-adapter"
	"github.com/ashrafAli23/nestgo/conformance"
	core "github.com/ashrafAli23/nestgo/core"
)

func TestConformance(t *testing.T) {
	conformance.Run(t, func(cfg *core.Config) core.Server {
		return ginadapter.New(cfg)
	})
}

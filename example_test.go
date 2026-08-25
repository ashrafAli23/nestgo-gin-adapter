package ginadapter_test

import (
	"fmt"

	ginadapter "github.com/ashrafAli23/nestgo-gin-adapter"
	core "github.com/ashrafAli23/nestgo/core"
)

func ExampleNew() {
	cfg := core.DefaultConfig()
	cfg.DisableLogger = true
	server := ginadapter.New(cfg)
	server.GET("/hello", func(c core.Context) error {
		return c.JSON(200, map[string]string{"message": "hello"})
	})
	fmt.Println(server.Name())
	// Output: gin
}

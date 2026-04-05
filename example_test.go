package ginadapter_test

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ashrafAli23/nestgo/core"
	ginadapter "github.com/ashrafAli23/nestgo-gin-adapter"
)

func ExampleNew() {
	// Create a Gin-backed NestGo server with default config.
	server := ginadapter.New(core.DefaultConfig())

	server.GET("/ping", func(c core.Context) error {
		return c.JSON(200, map[string]string{"pong": "true"})
	})

	// server.Start(":3000")
	fmt.Println("server type:", server.Name())
	// Output: server type: gin
}

func ExampleNew_customConfig() {
	// Custom config with timeouts, release mode, and body limit.
	config := &core.Config{
		AppName:      "my-api",
		Addr:         ":8080",
		Debug:        false,
		ReadTimeout:  30,
		WriteTimeout: 30,
		BodyLimit:    10 * 1024 * 1024, // 10MB
	}

	server := ginadapter.New(config)
	fmt.Println("server type:", server.Name())
	// Output: server type: gin
}

func ExampleNew_disableLogger() {
	// Create server without Gin's default logger middleware.
	// Useful when you have your own structured logging.
	server := ginadapter.New(&core.Config{
		DisableLogger: true,
	})

	fmt.Println("server type:", server.Name())
	// Output: server type: gin
}

func ExampleGinServer_Group() {
	server := ginadapter.New(core.DefaultConfig())

	// Create a route group with a prefix.
	api := server.Group("/api/v1")
	api.GET("/users", func(c core.Context) error {
		return c.JSON(200, []string{"alice", "bob"})
	})
	api.POST("/users", func(c core.Context) error {
		return c.JSON(201, map[string]string{"created": "true"})
	})

	fmt.Println("server type:", server.Name())
	// Output: server type: gin
}

func ExampleGinServer_Underlying() {
	server := ginadapter.New(core.DefaultConfig())

	// Access the raw *gin.Engine for Gin-specific features.
	// engine := server.Underlying().(*gin.Engine)
	// engine.SetTrustedProxies([]string{"192.168.1.0/24"})
	// engine.LoadHTMLGlob("templates/*")
	_ = server.Underlying()

	fmt.Println("underlying available:", server.Underlying() != nil)
	// Output: underlying available: true
}

func ExampleGinServer_Shutdown() {
	server := ginadapter.New(core.DefaultConfig())

	// Graceful shutdown with OS signal handling.
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()

	// server.Start(":3000")
	fmt.Println("shutdown handler registered")
	// Output: shutdown handler registered
}

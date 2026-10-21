// Command http is the HTTP API: a Lambda behind API Gateway, or, on a
// developer's machine, a local server that also relays board sockets.
package main

import (
	"github.com/Schiffer116/skipli/api/internal/config"
	"github.com/Schiffer116/skipli/api/internal/server"
)

func main() {
	c := config.Load()
	serve(c, server.NewRouter(c))
}

package main

import (
	"context"
	"strings"
	"testing"

	"github.com/massi/localwhatsappmcp/internal/service"
)

func TestServeRejectsNonLoopbackAddresses(t *testing.T) {
	for _, address := range []string{"0.0.0.0:32191", "[::]:32191", "192.0.2.1:32191", "bridge.example:32191", "localhost:32191"} {
		t.Run(address, func(t *testing.T) {
			err := serve(context.Background(), service.Options{DataDir: t.TempDir()}, "/unused", address, testControlToken)
			if err == nil || !strings.Contains(err.Error(), "explicit loopback IP") {
				t.Fatalf("non-loopback listener was not rejected: %v", err)
			}
		})
	}
}

func TestRemovedCommandsCannotRun(t *testing.T) {
	for _, command := range []string{"cloud", "cloud-authorize", "tunnel-worker", "setup", "headers", "doctor", "stdio"} {
		if err := run([]string{command}); err == nil || err.Error() != "unknown service command" {
			t.Fatalf("removed command %s was recognized: %v", command, err)
		}
	}
}

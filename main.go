package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/kubernetes-sigs-issues/iac/kwok/internal/app"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/cli"
)

func main() {
	flags, cfg := cli.MustParse(os.Args[1:])

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := app.Run(ctx, flags, cfg); err != nil {
		log.Fatalf("%v", err)
	}
}

package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/go-logr/logr"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/app"
	"github.com/kubernetes-sigs-issues/iac/kwok/internal/cli"
	crlog "sigs.k8s.io/controller-runtime/pkg/log"
)

func init() {
	// controller-runtime requires a logr logger to be set; silence it since
	// external-dns already logs everything relevant through logrus.
	crlog.SetLogger(logr.Discard())
}

func main() {
	flags, cfg := cli.MustParse(os.Args[1:])

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	if err := app.Run(ctx, flags, cfg); err != nil {
		log.Fatalf("%v", err)
	}
}

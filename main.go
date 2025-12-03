package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/cloudpilot-ai/terraform-provider-cloudpilotai/pkg/provider"
)

var version string = "dev"

func main() {
	ctx := context.Background()
	var debug bool

	flag.BoolVar(&debug, "debug", false, "set to true to run the provider with support for debuggers like delve")
	flag.Parse()

	opts := providerserver.ServeOpts{
		Address: "registry.terraform.io/cloudpilot-ai/cloudpilotai",
		Debug:   debug,
	}

	err := providerserver.Serve(ctx, provider.NewProvider(version), opts)
	if err != nil {
		log.Fatal(err.Error())
	}
}

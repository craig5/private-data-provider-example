package main

import (
	"flag"

	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"

	"github.com/craig5/private-data-provider-example/provider/internal/provider"
)

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run with support for debuggers like delve")
	flag.Parse()

	plugin.Serve(&plugin.ServeOpts{
		ProviderFunc: provider.New,
		ProviderAddr: "providers.example.internal/craig5/c5egprov",
		Debug:        debug,
	})
}

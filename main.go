// terraform-provider-capydb is the Terraform/OpenTofu provider for CapyDB
// managed Postgres hosting (capydb.dev).
package main

import (
	"context"
	"flag"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"

	"github.com/capy-base/terraform-provider-capydb/internal/provider"
)

// version is set by GoReleaser at build time via ldflags.
var version = "dev"

func main() {
	var debug bool
	flag.BoolVar(&debug, "debug", false, "run the provider with debugger support (e.g. delve)")
	flag.Parse()

	err := providerserver.Serve(context.Background(), provider.New(version), providerserver.ServeOpts{
		Address: "registry.terraform.io/capy-base/capydb",
		Debug:   debug,
	})
	if err != nil {
		log.Fatal(err)
	}
}

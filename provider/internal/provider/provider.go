// Package provider is the OpenTofu/Terraform glue, written with the Plugin
// SDK v2. It holds no parsing logic; see internal/model for that.
package provider

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// New returns the c5egprov provider. It is data-only: no resources, no
// credentials, no API calls.
//
// The name is squashed on purpose. "_" is not allowed in a provider name, since
// it separates the provider from the resource in names like
// c5egprov_directory, and some private registries reject "-" as well. The "c5"
// prefix is the org, so it is unlikely to collide with another provider's
// resources.
func New() *schema.Provider {
	return &schema.Provider{
		DataSourcesMap: map[string]*schema.Resource{
			"c5egprov_directory": dataSourceDirectory(),
		},
	}
}

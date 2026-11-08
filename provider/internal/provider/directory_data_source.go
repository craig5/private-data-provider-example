package provider

import (
	"context"
	"maps"
	"slices"

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/craig5/private-data-provider-example/provider/internal/model"
)

// dataSourceDirectory is c5egprov_directory.
//
// SDKv2 can only return a map of primitive values, not a map of objects, so
// users, roles and bindings come back as lists, sorted by their key. The HCL
// turns each list back into a map for for_each.
func dataSourceDirectory() *schema.Resource {
	return &schema.Resource{
		Description: "Parses a data directory (roles.yaml plus teams/*.yaml) into users, roles and the bindings between them.",
		ReadContext: readDirectory,
		Schema: map[string]*schema.Schema{
			"data_dir": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Directory holding roles.yaml and a teams/ directory of team files.",
			},
			"email_domain": {
				Type:        schema.TypeString,
				Required:    true,
				Description: "Domain for users with no explicit email.",
			},
			"users": computedList("Enabled users, sorted by username.", map[string]*schema.Schema{
				"username":  computedString(),
				"full_name": computedString(),
				"email":     computedString(),
				"team":      computedString(),
				"roles": {
					Type:     schema.TypeList,
					Computed: true,
					Elem:     &schema.Schema{Type: schema.TypeString},
				},
			}),
			"roles": computedList("Every role, sorted by name.", map[string]*schema.Schema{
				"name":        computedString(),
				"description": computedString(),
			}),
			"bindings": computedList("One entry per user and role, sorted by \"username/role\".", map[string]*schema.Schema{
				"user": computedString(),
				"role": computedString(),
			}),
		},
	}
}

func computedString() *schema.Schema {
	return &schema.Schema{Type: schema.TypeString, Computed: true}
}

func computedList(description string, fields map[string]*schema.Schema) *schema.Schema {
	return &schema.Schema{
		Type:        schema.TypeList,
		Computed:    true,
		Description: description,
		Elem:        &schema.Resource{Schema: fields},
	}
}

func readDirectory(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
	dataDir := d.Get("data_dir").(string)
	dir, err := model.Load(dataDir, d.Get("email_domain").(string))
	if err != nil {
		// Attached to data_dir, so OpenTofu points at that line of the config.
		return diag.Diagnostics{{
			Severity:      diag.Error,
			Summary:       "Invalid data directory",
			Detail:        err.Error(),
			AttributePath: cty.GetAttrPath("data_dir"),
		}}
	}

	var users, roles, bindings []any
	for _, k := range slices.Sorted(maps.Keys(dir.Users)) {
		u := dir.Users[k]
		userRoles := make([]any, len(u.Roles))
		for i, r := range u.Roles {
			userRoles[i] = r
		}
		users = append(users, map[string]any{
			"username":  u.Username,
			"full_name": u.FullName,
			"email":     u.Email,
			"team":      u.Team,
			"roles":     userRoles,
		})
	}
	for _, k := range slices.Sorted(maps.Keys(dir.Roles)) {
		r := dir.Roles[k]
		roles = append(roles, map[string]any{"name": r.Name, "description": r.Description})
	}
	for _, k := range slices.Sorted(maps.Keys(dir.Bindings)) {
		b := dir.Bindings[k]
		bindings = append(bindings, map[string]any{"user": b.User, "role": b.Role})
	}

	for name, value := range map[string]any{"users": users, "roles": roles, "bindings": bindings} {
		if err := d.Set(name, value); err != nil {
			return diag.FromErr(err)
		}
	}

	// Every data source needs an ID; the directory it read will do.
	d.SetId(dataDir)
	return nil
}

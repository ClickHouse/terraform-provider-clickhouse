package clickhouse

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

func Provider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			// "credentials_file": {
			// 	Type:     schema.TypeString,
			// 	Required: true,
			// },
			"environment": {
				Type:     schema.TypeString,
				Optional: true,
				Default:  "production",
			},
			"organization_id": {
				Type:     schema.TypeString,
				Required: true,
			},
			"token_key": {
				Type:     schema.TypeString,
				Required: true,
			},
			"token_secret": {
				Type:     schema.TypeString,
				Required: true,
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"clickhouse_service": initServiceAllocationSchema(),
		},
		DataSourcesMap:       map[string]*schema.Resource{},
		ConfigureContextFunc: providerContextConfigure,
	}
}

func readTokenFromFile(filePath string) (string, string) {
	return "avhj1U5QCdWAE9CA9", "4b1dROiHQEuSXJHlV8zHFd0S7WQj7CGxz5kGJeJnca"
}

func providerContextConfigure(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	// tokenKey, tokenSecret := readTokenFromFile(d.Get("credentials_file").(string))
	env := d.Get("environment").(string)
	organizationId := d.Get("organization_id").(string)
	tokenKey := d.Get("token_key").(string)
	tokenSecret := d.Get("token_secret").(string)
	c, err := NewClient(env, organizationId, tokenKey, tokenSecret)
	if err != nil {
		return nil, diag.FromErr(err)
	}

	return c, diags
}

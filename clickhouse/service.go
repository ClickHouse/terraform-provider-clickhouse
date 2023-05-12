package clickhouse

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// initServiceAllocationSchema is where we define the schema of the Terraform data source
func initServiceAllocationSchema() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceServiceCreate,
		ReadContext:   resourceServiceRead,
		UpdateContext: resourceServiceUpdate,
		DeleteContext: resourceServiceDelete,
		Schema: map[string]*schema.Schema{
			"id": {
				Type:     schema.TypeString,
				Computed: true,
			},
			"name": {
				Type:     schema.TypeString,
				Required: true,
			},
			"cloud_provider": {
				Type:     schema.TypeString,
				Required: true,
			},
			"region": {
				Type:     schema.TypeString,
				Required: true,
			},
			"tier": {
				Type:     schema.TypeString,
				Required: true,
			},
			"idle_scaling": {
				Type:     schema.TypeBool,
				Required: true,
			},
			"ip_access": {
				Type:     schema.TypeList,
				Required: true,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"source": {
							Type:     schema.TypeString,
							Required: true,
						},
						"description": {
							Type:     schema.TypeString,
							Optional: true,
						},
					},
				},
			},
			"min_total_memory_gb": {
				Type:     schema.TypeInt,
				Required: true,
			},
			"max_total_memory_gb": {
				Type:     schema.TypeInt,
				Required: true,
			},
			"idle_timeout_minutes": {
				Type:     schema.TypeInt,
				Required: true,
			},
		},
	}
}

func resourceServiceCreate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	ipAccessListRaw := d.Get("ip_access").([]interface{})
	ipAccessList := []IpAccess{}

	for _, item := range ipAccessListRaw {
		i := item.(map[string]interface{})

		ipAccess := IpAccess{
			Source:      i["source"].(string),
			Description: i["description"].(string),
		}

		ipAccessList = append(ipAccessList, ipAccess)
	}

	service := Service{
		Name:               d.Get("name").(string),
		Provider:           d.Get("cloud_provider").(string),
		Region:             d.Get("region").(string),
		Tier:               d.Get("tier").(string),
		IdleScaling:        d.Get("idle_scaling").(bool),
		IpAccessList:       ipAccessList,
		MinTotalMemoryGb:   d.Get("min_total_memory_gb").(int),
		MaxTotalMemoryGb:   d.Get("max_total_memory_gb").(int),
		IdleTimeoutMinutes: d.Get("idle_timeout_minutes").(int),
	}

	s, err := c.CreateService(service, diags)
	if err != nil {
		return diag.FromErr(err)
	}

	d.SetId(s.Id)

	resourceServiceRead(ctx, d, m)

	return diags
}

func resourceServiceRead(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	serviceId := d.Id()

	_, err := c.GetService(serviceId, diags)
	if err != nil {
		return diag.FromErr(err)
	}

	return diags
}

func resourceServiceUpdate(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	serviceId := d.Id()
	service := ServiceUpdate{
		Name:         d.Get("name").(string),
		IpAccessList: nil,
	}

	if d.HasChange("ip_access") {
		ipAccessListRawOld, ipAccessListRawNew := d.GetChange("ip_access")
		ipAccessListOld := []IpAccess{}
		ipAccessListNew := []IpAccess{}

		for _, item := range ipAccessListRawOld.([]interface{}) {
			i := item.(map[string]interface{})

			ipAccess := IpAccess{
				Source:      i["source"].(string),
				Description: i["description"].(string),
			}

			ipAccessListOld = append(ipAccessListOld, ipAccess)
		}

		for _, item := range ipAccessListRawNew.([]interface{}) {
			i := item.(map[string]interface{})

			ipAccess := IpAccess{
				Source:      i["source"].(string),
				Description: i["description"].(string),
			}

			ipAccessListNew = append(ipAccessListNew, ipAccess)
		}

		add, remove := diffArrays(ipAccessListOld, ipAccessListNew, func(a IpAccess) string {
			return a.Source
		})

		service.IpAccessList = &IpAccessUpdate{
			Add:    add,
			Remove: remove,
		}
	}

	_, err := c.UpdateService(serviceId, service, diags)
	if err != nil {
		return diag.FromErr(err)
	}

	return diags
}

func resourceServiceDelete(ctx context.Context, d *schema.ResourceData, m interface{}) diag.Diagnostics {
	c := m.(*Client)

	// Warning or errors can be collected in a slice type
	var diags diag.Diagnostics

	serviceId := d.Id()

	_, err := c.DeleteService(serviceId, diags)
	if err != nil {
		return diag.FromErr(err)
	}

	return diags
}

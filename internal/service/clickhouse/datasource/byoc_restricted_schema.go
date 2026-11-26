package datasource

import (
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

func byocAccessModelDataSourceSchema() schema.StringAttribute {
	return schema.StringAttribute{
		Description: "Access model of the infrastructure: 'standard' (ClickHouse provisions and operates the data plane through the onboarding role) or 'restricted' (the customer provisions the cluster; ClickHouse operates it only through the connector the customer installs).",
		Computed:    true,
	}
}

func byocComputedString(description string) schema.StringAttribute {
	return schema.StringAttribute{Description: description, Computed: true}
}

func byocComputedBool(description string) schema.BoolAttribute {
	return schema.BoolAttribute{Description: description, Computed: true}
}

func byocComputedStringList(description string) schema.ListAttribute {
	return schema.ListAttribute{Description: description, Computed: true, ElementType: types.StringType}
}

func byocRestrictedFactsDataSourceSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Description: "Registration of the customer-provisioned cluster of a restricted-access infrastructure. Null for standard infrastructures. Optional values left to the API default are null.",
		Computed:    true,
		Attributes: map[string]schema.Attribute{
			"cluster_name":               byocComputedString("Name of the EKS cluster the connector runs in."),
			"platform_node_group":        byocComputedString("Node group the platform components run on."),
			"availability_zones":         byocComputedStringList("Availability zones of the region the infrastructure uses."),
			"namespace_prefix":           byocComputedString("Prefix of the namespace each service runs in."),
			"executor_role_arn":          byocComputedString("Direct image mode only: IAM role ARN of the connector executor."),
			"node_role_arns":             byocComputedStringList("Direct image mode only: IAM role ARNs of the cluster nodes."),
			"image_mode":                 byocComputedString("How the connector pulls platform images: 'direct' from the ClickHouse registry, or 'bundle' from 'image_registry'."),
			"image_registry":             byocComputedString("Bundle image mode only: OCI repository root holding the platform images."),
			"data_bucket":                byocComputedString("S3 bucket services store data in."),
			"backup_bucket":              byocComputedString("S3 bucket services store backups in."),
			"storage_role_arn":           byocComputedString("IAM role ARN services assume to reach the buckets."),
			"scraper_endpoint":           byocComputedString("Remote-write URL of the connector scraper."),
			"enable_monitoring":          byocComputedBool("Whether the monitoring components are installed."),
			"enable_snapshot_controller": byocComputedBool("Whether the volume snapshot controller is installed."),
			"enable_autoscaler":          byocComputedBool("Whether the ClickHouse autoscaler is installed."),
		},
	}
}

func byocConnectorDataSourceSchema() schema.SingleNestedAttribute {
	return schema.SingleNestedAttribute{
		Description: "Liveness of the connector of a restricted-access infrastructure, as reported when the data source was read. Null for standard infrastructures.",
		Computed:    true,
		Attributes: map[string]schema.Attribute{
			"last_heartbeat_at": byocComputedString("Last heartbeat of the connector (ISO-8601). Null until the connector first reports."),
			"platform_state":    byocComputedString("State of the platform layer the connector installs."),
			"platform_detail":   byocComputedString("What the customer needs to do when the platform layer waits on them."),
			"components": schema.ListNestedAttribute{
				Description: "Components the connector reports on.",
				Computed:    true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"name":              byocComputedString("Connector component, e.g. 'scraper', 'executor' or 'troubleshooter'."),
						"state":             byocComputedString("State the component last reported."),
						"version":           byocComputedString("Version the component last reported."),
						"last_heartbeat_at": byocComputedString("Last heartbeat of the component (ISO-8601)."),
					},
				},
			},
		},
	}
}

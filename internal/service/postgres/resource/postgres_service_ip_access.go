package resource

import (
	"context"
	"fmt"
	"net/netip"
	"strconv"
	"strings"
	"time"

	"github.com/cenkalti/backoff/v4"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ClickHouse/terraform-provider-clickhouse/internal/api"
	"github.com/ClickHouse/terraform-provider-clickhouse/internal/service/postgres/resource/models"
)

func postgresIPAccessSchema() schema.ListNestedAttribute {
	return schema.ListNestedAttribute{
		Description:   "IPv4 TCP access list. Omit to preserve existing rules or use the public defaults on creation. An empty list blocks public access on standard creation; on update it removes IPv4 rules only. IPv6 rules are not managed.",
		Optional:      true,
		Computed:      true,
		PlanModifiers: []planmodifier.List{listplanmodifier.UseStateForUnknown()},
		NestedObject: schema.NestedAttributeObject{Attributes: map[string]schema.Attribute{
			"source": schema.StringAttribute{
				Description: "IPv4 address or CIDR allowed to connect. Use 0.0.0.0/0 to allow all IPv4 addresses.",
				Required:    true,
			},
			"port_range": schema.StringAttribute{
				Description: "TCP port or inclusive range from 1 to 65535. Use 5432 for Postgres and 6432 for PgBouncer, or a range such as 5432..6432.",
				Required:    true,
			},
			"description": schema.StringAttribute{Description: "Description of the IP address.", Required: true},
		}},
	}
}

func postgresIPSource(source string) (string, error) {
	prefix, err := netip.ParsePrefix(source)
	if err != nil {
		address, addressErr := netip.ParseAddr(source)
		if addressErr != nil || !address.Is4() {
			return "", fmt.Errorf("expected an IPv4 address or CIDR")
		}
		prefix = netip.PrefixFrom(address, 32)
	}
	if !prefix.Addr().Is4() {
		return "", fmt.Errorf("expected an IPv4 address or CIDR")
	}
	return prefix.Masked().String(), nil
}

func postgresIPPortRange(value string, minPort int) (string, error) {
	parts := strings.Split(value, "..")
	if len(parts) > 2 {
		return "", fmt.Errorf("expected a port or inclusive range from %d to 65535", minPort)
	}
	ports := make([]int, len(parts))
	for i, part := range parts {
		if part == "" || strings.IndexFunc(part, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
			return "", fmt.Errorf("expected a port or inclusive range from %d to 65535", minPort)
		}
		port, err := strconv.Atoi(part)
		if err != nil || port < minPort || port > 65535 {
			return "", fmt.Errorf("expected a port or inclusive range from %d to 65535", minPort)
		}
		ports[i] = port
	}
	if len(ports) == 1 || ports[0] == ports[1] {
		return strconv.Itoa(ports[0]), nil
	}
	if ports[0] > ports[1] {
		return "", fmt.Errorf("the start port must not exceed the end port")
	}
	return fmt.Sprintf("%d..%d", ports[0], ports[1]), nil
}

func validatePostgresIPAccess(list types.List) diag.Diagnostics {
	var diags diag.Diagnostics
	seen := make(map[string]bool)
	for i, element := range list.Elements() {
		p := path.Root("ip_access").AtListIndex(i)
		if element.IsNull() {
			diags.AddAttributeError(p, "Invalid IP access entry", "Access list entries must not be null.")
			continue
		}
		if element.IsUnknown() {
			continue
		}
		attrs := element.(types.Object).Attributes()
		source := attrs["source"].(types.String)
		port := attrs["port_range"].(types.String)
		var canonicalSource, canonicalPort string
		if !source.IsUnknown() && !source.IsNull() {
			value, err := postgresIPSource(source.ValueString())
			if err != nil {
				diags.AddAttributeError(p.AtName("source"), "Invalid IPv4 source", err.Error())
			}
			canonicalSource = value
		}
		if !port.IsUnknown() && !port.IsNull() {
			value, err := postgresIPPortRange(port.ValueString(), 1)
			if err != nil {
				diags.AddAttributeError(p.AtName("port_range"), "Invalid port range", err.Error())
			}
			canonicalPort = value
		}
		if canonicalSource != "" && canonicalPort != "" {
			key := canonicalSource + ":" + canonicalPort
			if seen[key] {
				diags.AddAttributeError(p, "Duplicate IP access entry", "Each IPv4 source and port range combination must be unique.")
			}
			seen[key] = true
		}
	}
	return diags
}

func planPostgresIPAccess(ctx context.Context, list types.List) (*[]api.PostgresIpAccess, diag.Diagnostics) {
	var diags diag.Diagnostics
	if list.IsNull() || list.IsUnknown() {
		return nil, diags
	}
	var entries []models.IPAccessList
	diags.Append(list.ElementsAs(ctx, &entries, false)...)
	rules := make([]api.PostgresIpAccess, 0, len(entries))
	for _, entry := range entries {
		if entry.Source.IsUnknown() || entry.PortRange.IsUnknown() || entry.Description.IsUnknown() ||
			entry.Source.IsNull() || entry.PortRange.IsNull() || entry.Description.IsNull() {
			diags.AddError("Unresolved IP access entry", "Source, port_range, and description must be known before applying IP access changes.")
			continue
		}
		rules = append(rules, api.PostgresIpAccess{
			Source: entry.Source.ValueString(), PortRange: entry.PortRange.ValueString(), Description: entry.Description.ValueString(),
		})
	}
	return &rules, diags
}

func postgresIPAccessKey(rule api.PostgresIpAccess) (string, error) {
	source, err := postgresIPSource(rule.Source)
	if err != nil {
		return "", err
	}
	// Existing backend rules may include port zero even though new configurations cannot.
	port, err := postgresIPPortRange(rule.PortRange, 0)
	if err != nil {
		return "", err
	}
	return source + ":" + port, nil
}

func trimPostgresIPAccessDescription(description string) string {
	// Match Ubicloud's Ruby String#strip, which preserves Unicode whitespace.
	return strings.Trim(description, "\x00\t\n\v\f\r ")
}

func diffPostgresIPAccess(current, desired []api.PostgresIpAccess) (*api.PostgresIpAccessUpdate, error) {
	patch := &api.PostgresIpAccessUpdate{}
	byKey := make(map[string][]api.PostgresIpAccess, len(current))
	for _, rule := range current {
		key, err := postgresIPAccessKey(rule)
		if err != nil || rule.Id == "" {
			return nil, fmt.Errorf("the API returned an invalid IP access entry or a missing rule ID")
		}
		byKey[key] = append(byKey[key], rule)
	}
	kept := make(map[string]bool, len(desired))
	for _, rule := range desired {
		key, err := postgresIPAccessKey(rule)
		if err != nil {
			return nil, err
		}
		matches := byKey[key]
		if len(matches) == 0 {
			patch.Add = append(patch.Add, rule)
			continue
		}
		match := matches[0]
		byKey[key] = matches[1:]
		kept[match.Id] = true
		if match.Description != trimPostgresIPAccessDescription(rule.Description) {
			match.Description = rule.Description
			patch.Add = append(patch.Add, match)
		}
	}
	for _, rule := range current {
		if !kept[rule.Id] {
			patch.Remove = append(patch.Remove, api.PostgresIpAccessRuleReference{Id: rule.Id})
		}
	}
	if len(patch.Add) == 0 && len(patch.Remove) == 0 {
		return nil, nil
	}
	return patch, nil
}

func (r *PostgresServiceResource) updatePostgresIPAccess(ctx context.Context, id string, desired types.List) error {
	rules, diags := planPostgresIPAccess(ctx, desired)
	if diags.HasError() {
		return fmt.Errorf("invalid IP access list: %v", diags)
	}
	if rules == nil {
		return nil
	}
	update := func() error {
		if err := ctx.Err(); err != nil {
			return backoff.Permanent(err)
		}
		current, err := r.client.GetPostgres(ctx, id)
		if err != nil {
			// GET already retries safely in the HTTP client.
			return backoff.Permanent(err)
		}
		if current.IpAccessList == nil {
			return backoff.Permanent(fmt.Errorf("the API did not return ipAccessList; cannot safely update IP access rules"))
		}
		patch, err := diffPostgresIPAccess(current.IpAccessList, *rules)
		if err != nil || patch == nil {
			return backoff.Permanent(err)
		}
		// Keep this separate from metadata changes: an access-only PATCH preserves
		// tags and is allowed on read replicas.
		_, err = r.client.UpdatePostgres(ctx, id, api.PostgresUpdate{IpAccessList: patch})
		if api.Is4xxPermanent(err) {
			return backoff.Permanent(err)
		}
		return err
	}
	// A failed response may follow a partial or complete mutation. Re-read and
	// diff before retrying so deleted IDs and completed additions are not replayed.
	return backoff.Retry(update, backoff.WithContext(backoff.NewExponentialBackOff(
		backoff.WithMaxElapsedTime(time.Minute),
	), ctx))
}

func syncPostgresIPAccess(ctx context.Context, rules []api.PostgresIpAccess, prior types.List) (types.List, diag.Diagnostics) {
	elementType := models.IPAccessList{}.ObjectType()
	var diags diag.Diagnostics
	if rules == nil {
		if !prior.IsNull() && !prior.IsUnknown() {
			diags.AddError("IP access list missing", "The API did not return ipAccessList; cannot refresh the managed IP access rules.")
		}
		return types.ListNull(elementType), diags
	}
	var previous []models.IPAccessList
	if !prior.IsNull() && !prior.IsUnknown() {
		diags.Append(prior.ElementsAs(ctx, &previous, false)...)
	}
	if diags.HasError() {
		return types.ListNull(elementType), diags
	}
	keys := make([]string, len(rules))
	for i, rule := range rules {
		key, err := postgresIPAccessKey(rule)
		if err != nil {
			diags.AddError("Invalid IP access response", err.Error())
			return types.ListNull(elementType), diags
		}
		keys[i] = key
	}
	elements := make([]attr.Value, 0, len(rules))
	used := make([]bool, len(rules))
	for _, entry := range previous {
		key, err := postgresIPAccessKey(api.PostgresIpAccess{Source: entry.Source.ValueString(), PortRange: entry.PortRange.ValueString()})
		if err != nil {
			continue
		}
		for i, rule := range rules {
			if used[i] || key != keys[i] {
				continue
			}
			used[i] = true
			if trimPostgresIPAccessDescription(entry.Description.ValueString()) != rule.Description {
				entry.Description = types.StringValue(rule.Description)
			}
			elements = append(elements, entry.ObjectValue())
			break
		}
	}
	for i, rule := range rules {
		if !used[i] {
			elements = append(elements, models.IPAccessList{
				Source: types.StringValue(rule.Source), PortRange: types.StringValue(rule.PortRange), Description: types.StringValue(rule.Description),
			}.ObjectValue())
		}
	}
	return types.ListValue(elementType, elements)
}

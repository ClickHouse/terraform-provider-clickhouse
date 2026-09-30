package clickstack

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/types"
)

const tagsAllAttr = "tags_all"

// tagsAllAttribute is the computed tags_all shared by every taggable resource.
// It is a set so a server that reorders tags does not plan an update.
func tagsAllAttribute(extra string) schema.SetAttribute {
	return schema.SetAttribute{
		ElementType: types.StringType,
		Computed:    true,
		Description: "All tags on the resource: its own tags plus the provider's `clickstack_default_tags`." + extra,
	}
}

// maxTags is the API's per-resource tag limit.
const maxTags = 50

// plannedTagsAll is tags_all for a plan, rejecting a merge the API would refuse.
func plannedTagsAll(own, defaults []string) (types.Set, diag.Diagnostics) {
	all, diags := tagsSet(withDefaultTags(own, defaults))
	if n := len(all.Elements()); n > maxTags {
		diags.AddAttributeError(path.Root(tagsAllAttr), "Too many tags",
			fmt.Sprintf("the resource's own tags plus clickstack_default_tags come to %d; ClickStack allows %d", n, maxTags))
	}
	return all, diags
}

// fullyKnown reports whether a list and every element in it are known.
func fullyKnown(l types.List) bool {
	return known(l) && !slices.ContainsFunc(l.Elements(), func(v attr.Value) bool { return v.IsUnknown() })
}

// withDefaultTags returns own followed by each default it does not already list.
func withDefaultTags(own, defaults []string) []string {
	out := slices.Clone(own)
	for _, t := range defaults {
		if !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

// withoutDefaultTags drops the tags in drop that the resource did not list
// itself, so provider defaults never show up as drift on its own tags.
func withoutDefaultTags(server, own, drop []string) []string {
	out := []string{}
	for _, t := range server {
		if slices.Contains(own, t) || !slices.Contains(drop, t) {
			out = append(out, t)
		}
	}
	return out
}

// tagsDropList is what a read strips from the server's tags: current defaults plus
// the last apply's tags_all, so a default since removed from the provider does not leak in.
func tagsDropList(ctx context.Context, priorAll types.Set, defaults []string) ([]string, diag.Diagnostics) {
	var prior []string
	var diags diag.Diagnostics
	if known(priorAll) {
		diags = priorAll.ElementsAs(ctx, &prior, false)
	}
	return append(prior, defaults...), diags
}

// tagsSet builds tags_all. A set rejects duplicates, which an authored tags list may have.
func tagsSet(tags []string) (types.Set, diag.Diagnostics) {
	tags = slices.Compact(slices.Sorted(slices.Values(tags)))
	elems := make([]attr.Value, len(tags))
	for i, t := range tags {
		elems[i] = types.StringValue(t)
	}
	return types.SetValue(types.StringType, elems)
}

// dashboardTags reads the tags array of a dashboard body.
func dashboardTags(body []byte) ([]string, error) {
	var d struct {
		Tags []string `json:"tags"`
	}
	if err := json.Unmarshal(body, &d); err != nil {
		return nil, fmt.Errorf("read dashboard tags: %w", err)
	}
	return d.Tags, nil
}

// setDashboardTags replaces the tags array of a dashboard body.
func setDashboardTags(body []byte, tags []string) ([]byte, error) {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("set dashboard tags: %w", err)
	}
	raw, err := json.Marshal(tags)
	if err != nil {
		return nil, fmt.Errorf("set dashboard tags: %w", err)
	}
	doc["tags"] = raw
	return json.Marshal(doc)
}

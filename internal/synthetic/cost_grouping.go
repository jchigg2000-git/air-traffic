package synthetic

// cost_grouping.go provides the byte-identical drill-down machinery shared by every
// vendor fixture: when a cost/usage request carries the vendor's REAL group-by param, the
// fixture emits one grouped result row per catalog facet member, reading the SAME
// catalog.CostFacets the emitter attributes spend against. Same envelope, real field names,
// multiple rows — byte-for-byte compatible with each vendor's actual grouped response.

import (
	"strings"

	"air-traffic/internal/catalog"
)

// facetByField returns the supported cost facet a vendor groups cost/usage by under the
// given response field (e.g. "project_id", "model", "workspace_id"), or ok=false.
func facetByField(vendorID, field string) (catalog.CostFacet, bool) {
	for _, f := range catalog.CostFacetsFor(vendorID) {
		if f.Supported && len(f.Members) > 0 && f.ResponseField == field {
			return f, true
		}
	}
	return catalog.CostFacet{}, false
}

// requestedGroupField returns the first group-by field present in the query for any of the
// given param names, tolerating repeated params (?group_by=a&group_by=b), bracketed array
// form (?group_by[]=a), and comma-joined values (?group_by=a,b). Returns "" if none.
func requestedGroupField(q map[string][]string, params ...string) string {
	for _, p := range params {
		for _, key := range []string{p, p + "[]"} {
			for _, v := range q[key] {
				for _, part := range strings.Split(v, ",") {
					if s := strings.TrimSpace(part); s != "" {
						return s
					}
				}
			}
		}
	}
	return ""
}

// memberShare is one facet member with its normalized share of the vendor's spend.
type memberShare struct {
	Member catalog.FacetMember
	Share  float64
}

// shares normalizes a facet's member weights so they sum to 1 (stable ordering preserved).
func shares(f catalog.CostFacet) []memberShare {
	var total float64
	for _, m := range f.Members {
		total += m.Weight
	}
	if total <= 0 {
		return nil
	}
	out := make([]memberShare, 0, len(f.Members))
	for _, m := range f.Members {
		out = append(out, memberShare{Member: m, Share: m.Weight / total})
	}
	return out
}

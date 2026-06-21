// Command gcp fetches Compute Engine machine types from the GCP API and writes
// the catalog JSON to the specified output file.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	compute "cloud.google.com/go/compute/apiv1"
	computepb "cloud.google.com/go/compute/apiv1/computepb"
	"github.com/sgbudje/runright/internal/types"
	"google.golang.org/api/iterator"
)

func main() {
	output := flag.String("output", "catalog/data/gcp.json", "Output JSON file path")
	project := flag.String("project", "", "GCP project ID (required)")
	zone := flag.String("zone", "us-central1-a", "GCP zone to query")
	flag.Parse()

	if *project == "" {
		fmt.Fprintln(os.Stderr, "--project is required")
		os.Exit(1)
	}

	ctx := context.Background()
	client, err := compute.NewMachineTypesRESTClient(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "GCP client: %v\n", err)
		os.Exit(1)
	}
	defer client.Close()

	req := &computepb.ListMachineTypesRequest{
		Project: *project,
		Zone:    *zone,
	}

	it := client.List(ctx, req)
	var machines []types.MachineType

	for {
		mt, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "List machine types: %v\n", err)
			os.Exit(1)
		}
		m := convertGCP(mt)
		if m != nil {
			machines = append(machines, *m)
		}
	}

	f, err := os.Create(*output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Create output file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	if err := enc.Encode(machines); err != nil {
		fmt.Fprintf(os.Stderr, "Encode: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Wrote %d GCP machine types to %s\n", len(machines), *output)
}

func convertGCP(mt *computepb.MachineType) *types.MachineType {
	if mt == nil || mt.Name == nil {
		return nil
	}
	name := *mt.Name
	vcpus := 0
	if mt.GuestCpus != nil {
		vcpus = int(*mt.GuestCpus)
	}
	memGiB := 0.0
	if mt.MemoryMb != nil {
		memGiB = float64(*mt.MemoryMb) / 1024.0
	}

	family := classifyGCPFamily(name)
	series := extractGCPSeries(name)
	tags := buildGCPTags(name)

	return &types.MachineType{
		ID:           name,
		Provider:     types.ProviderGCP,
		Family:       family,
		Series:       series,
		VCPUs:        vcpus,
		MemoryGiB:    memGiB,
		NetworkGbps:  0, // Not available directly in this API response
		StorageType:  "pd",
		Architecture: "x86_64",
		Tags:         tags,
		// Pricing requires the Cloud Billing API; leave at 0 — static catalog has prices.
		OnDemandPricePerHour: 0,
	}
}

func extractGCPSeries(name string) string {
	parts := strings.SplitN(name, "-", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return name
}

func classifyGCPFamily(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasPrefix(lower, "e2") || strings.HasPrefix(lower, "n1") ||
		strings.HasPrefix(lower, "n2") || strings.HasPrefix(lower, "n4") ||
		strings.HasPrefix(lower, "t2"):
		return "general-purpose"
	case strings.HasPrefix(lower, "c2") || strings.HasPrefix(lower, "c3") ||
		strings.HasPrefix(lower, "c4") || strings.HasPrefix(lower, "h3"):
		return "compute-optimized"
	case strings.HasPrefix(lower, "m1") || strings.HasPrefix(lower, "m2") ||
		strings.HasPrefix(lower, "m3") || strings.HasPrefix(lower, "m4") ||
		strings.HasPrefix(lower, "x4"):
		return "memory-optimized"
	case strings.HasPrefix(lower, "z3"):
		return "storage-optimized"
	case strings.HasPrefix(lower, "a2") || strings.HasPrefix(lower, "a3") ||
		strings.HasPrefix(lower, "g2"):
		return "accelerator-optimized"
	default:
		return "general-purpose"
	}
}

func buildGCPTags(name string) []string {
	var tags []string
	lower := strings.ToLower(name)
	if strings.Contains(lower, "highcpu") {
		tags = append(tags, "highcpu")
	}
	if strings.Contains(lower, "highmem") {
		tags = append(tags, "highmem")
	}
	if strings.Contains(lower, "standard") {
		tags = append(tags, "standard")
	}
	if strings.HasPrefix(lower, "e2") {
		tags = append(tags, "low-cost")
	}
	return tags
}

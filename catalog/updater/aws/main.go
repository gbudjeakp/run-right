// Command aws fetches EC2 instance types from the AWS API and writes
// the catalog JSON to the specified output file.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/sgbudje/runright/internal/types"
)

func main() {
	output := flag.String("output", "catalog/data/aws.json", "Output JSON file path")
	region := flag.String("region", "us-east-1", "AWS region to query")
	flag.Parse()

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx, config.WithRegion(*region))
	if err != nil {
		fmt.Fprintf(os.Stderr, "AWS config: %v\n", err)
		os.Exit(1)
	}

	client := ec2.NewFromConfig(cfg)

	var machines []types.MachineType
	paginator := ec2.NewDescribeInstanceTypesPaginator(client, &ec2.DescribeInstanceTypesInput{
		Filters: []ec2types.Filter{
			{Name: strPtr("current-generation"), Values: []string{"true"}},
		},
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Describe instance types: %v\n", err)
			os.Exit(1)
		}
		for _, it := range page.InstanceTypes {
			m := convertAWS(it)
			if m != nil {
				machines = append(machines, *m)
			}
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
	fmt.Printf("Wrote %d AWS instance types to %s\n", len(machines), *output)
}

func convertAWS(it ec2types.InstanceTypeInfo) *types.MachineType {
	if it.InstanceType == "" || it.VCpuInfo == nil || it.MemoryInfo == nil {
		return nil
	}
	vcpus := 0
	if it.VCpuInfo.DefaultVCpus != nil {
		vcpus = int(*it.VCpuInfo.DefaultVCpus)
	}
	memGiB := float64(it.MemoryInfo.SizeInMiB) / 1024.0

	arch := "x86_64"
	if len(it.ProcessorInfo.SupportedArchitectures) > 0 {
		a := string(it.ProcessorInfo.SupportedArchitectures[0])
		if a == "arm64" {
			arch = "arm64"
		}
	}

	var networkGbps float64
	if it.NetworkInfo != nil && it.NetworkInfo.NetworkPerformance != nil {
		networkGbps = parseNetworkGbps(*it.NetworkInfo.NetworkPerformance)
	}

	storageType := "ebs"
	if it.InstanceStorageInfo != nil && it.InstanceStorageInfo.TotalSizeInGB != nil {
		storageType = "nvme"
	}

	id := string(it.InstanceType)
	family := classifyAWSFamily(id)
	series := extractAWSSeries(id)
	tags := buildAWSTags(it, series)

	return &types.MachineType{
		ID:           id,
		Provider:     types.ProviderAWS,
		Family:       family,
		Series:       series,
		VCPUs:        vcpus,
		MemoryGiB:    memGiB,
		NetworkGbps:  networkGbps,
		StorageType:  storageType,
		Architecture: arch,
		Tags:         tags,
		// Pricing requires a separate Pricing API call; leave at 0 for now.
		// The static catalog has accurate on-demand prices.
		OnDemandPricePerHour: 0,
	}
}

func classifyAWSFamily(id string) string {
	if len(id) == 0 {
		return "unknown"
	}
	switch id[0] {
	case 't':
		return "general-purpose"
	case 'm':
		return "general-purpose"
	case 'c':
		return "compute-optimized"
	case 'r', 'x', 'u':
		return "memory-optimized"
	case 'i', 'd', 'h':
		return "storage-optimized"
	case 'p', 'g', 'f', 'v':
		return "accelerated-computing"
	case 'a':
		return "general-purpose"
	default:
		return "other"
	}
}

func extractAWSSeries(id string) string {
	for i, c := range id {
		if c == '.' {
			return id[:i]
		}
	}
	return id
}

func buildAWSTags(it ec2types.InstanceTypeInfo, series string) []string {
	var tags []string
	id := string(it.InstanceType)
	if len(id) > 0 && id[0] == 't' {
		tags = append(tags, "burstable")
	}
	for _, arch := range it.ProcessorInfo.SupportedArchitectures {
		if arch == "arm64" {
			tags = append(tags, "graviton", "arm")
		}
	}
	_ = series
	return tags
}

func parseNetworkGbps(perf string) float64 {
	var gbps float64
	fmt.Sscanf(perf, "%f", &gbps)
	return gbps
}

func strPtr(s string) *string { return &s }

package catalog

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/sgbudje/runright/internal/types"
)

//go:embed data/aws.json
var awsData []byte

//go:embed data/gcp.json
var gcpData []byte

//go:embed data/github.json
var githubData []byte

var allMachines []types.MachineType

func init() {
	var aws []types.MachineType
	if err := json.Unmarshal(awsData, &aws); err != nil {
		panic(fmt.Sprintf("runright: failed to parse aws catalog: %v", err))
	}
	var gcp []types.MachineType
	if err := json.Unmarshal(gcpData, &gcp); err != nil {
		panic(fmt.Sprintf("runright: failed to parse gcp catalog: %v", err))
	}
	var github []types.MachineType
	if err := json.Unmarshal(githubData, &github); err != nil {
		panic(fmt.Sprintf("runright: failed to parse github catalog: %v", err))
	}
	allMachines = append(aws, gcp...)
	allMachines = append(allMachines, github...)
}

// All returns a copy of the full machine catalog.
func All() []types.MachineType {
	out := make([]types.MachineType, len(allMachines))
	copy(out, allMachines)
	return out
}

// QueryOptions allows filtering and sorting the catalog.
type QueryOptions struct {
	Provider        types.Provider
	MinVCPUs        int
	MaxVCPUs        int
	MinMemoryGiB    float64
	MaxMemoryGiB    float64
	MaxPricePerHour float64
	Architecture    string
	Tags            []string
}

// Query filters the catalog and returns matching machines sorted by price.
func Query(opts QueryOptions) []types.MachineType {
	var out []types.MachineType
	for _, m := range allMachines {
		if opts.Provider != "" && m.Provider != opts.Provider {
			continue
		}
		if opts.MinVCPUs > 0 && m.VCPUs < opts.MinVCPUs {
			continue
		}
		if opts.MaxVCPUs > 0 && m.VCPUs > opts.MaxVCPUs {
			continue
		}
		if opts.MinMemoryGiB > 0 && m.MemoryGiB < opts.MinMemoryGiB {
			continue
		}
		if opts.MaxMemoryGiB > 0 && m.MemoryGiB > opts.MaxMemoryGiB {
			continue
		}
		if opts.MaxPricePerHour > 0 && m.OnDemandPricePerHour > opts.MaxPricePerHour {
			continue
		}
		if opts.Architecture != "" && !strings.EqualFold(m.Architecture, opts.Architecture) {
			continue
		}
		if len(opts.Tags) > 0 && !hasTags(m, opts.Tags) {
			continue
		}
		out = append(out, m)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].OnDemandPricePerHour < out[j].OnDemandPricePerHour
	})
	return out
}

// FindByID returns a machine type by its ID, or nil if not found.
func FindByID(id string) *types.MachineType {
	for i := range allMachines {
		if allMachines[i].ID == id {
			return &allMachines[i]
		}
	}
	return nil
}

// DetectMachine matches vCPU count and total memory against the catalog to
// identify what machine the current runner is likely running on.
func DetectMachine(vcpus int, memTotalGiB float64) *types.MachineType {
	const memToleranceGiB = 2.0
	var best *types.MachineType
	for i := range allMachines {
		m := &allMachines[i]
		if m.VCPUs != vcpus {
			continue
		}
		diff := m.MemoryGiB - memTotalGiB
		if diff < 0 {
			diff = -diff
		}
		if diff <= memToleranceGiB {
			if best == nil || m.OnDemandPricePerHour < best.OnDemandPricePerHour {
				best = m
			}
		}
	}
	return best
}

func hasTags(m types.MachineType, required []string) bool {
	tagSet := make(map[string]struct{}, len(m.Tags))
	for _, t := range m.Tags {
		tagSet[t] = struct{}{}
	}
	for _, r := range required {
		if _, ok := tagSet[r]; !ok {
			return false
		}
	}
	return true
}

// Azure catalog updater - fetches current Azure VM pricing
// Run with: go run catalog/updater/azure/main.go > internal/catalog/data/azure.json
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strings"
)

type MachineType struct {
	ID                   string   `json:"id"`
	Provider             string   `json:"provider"`
	Family               string   `json:"family"`
	Series               string   `json:"series"`
	VCPUs                int      `json:"vcpus"`
	MemoryGiB            float64  `json:"memory_gib"`
	NetworkGbps          float64  `json:"network_gbps"`
	StorageType          string   `json:"storage_type"`
	Architecture         string   `json:"architecture"`
	OnDemandPricePerHour float64  `json:"on_demand_price_per_hour"`
	SpotPricePerHour     float64  `json:"spot_price_per_hour,omitempty"`
	Tags                 []string `json:"tags"`
}

// AzurePriceResponse represents the Azure Retail Prices API response
type AzurePriceResponse struct {
	Items       []AzurePriceItem `json:"Items"`
	NextPageLink string          `json:"NextPageLink"`
}

type AzurePriceItem struct {
	SkuName        string  `json:"skuName"`
	ProductName    string  `json:"productName"`
	ServiceFamily  string  `json:"serviceFamily"`
	ArmSkuName     string  `json:"armSkuName"`
	ArmRegionName  string  `json:"armRegionName"`
	RetailPrice    float64 `json:"retailPrice"`
	UnitOfMeasure  string  `json:"unitOfMeasure"`
	Type           string  `json:"type"`
	MeterName      string  `json:"meterName"`
}

func main() {
	// Note: For production, you would fetch from Azure Retail Prices API
	// https://prices.azure.com/api/retail/prices
	// This is a simplified version with static data

	machines := []MachineType{
		// Burstable B-series
		{ID: "Standard_B1s", Provider: "azure", Family: "burstable", Series: "B", VCPUs: 1, MemoryGiB: 1.0, NetworkGbps: 0.32, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.0104, SpotPricePerHour: 0.0021, Tags: []string{"burstable", "low-cost", "ci"}},
		{ID: "Standard_B1ms", Provider: "azure", Family: "burstable", Series: "B", VCPUs: 1, MemoryGiB: 2.0, NetworkGbps: 0.64, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.0207, SpotPricePerHour: 0.0041, Tags: []string{"burstable", "low-cost", "ci"}},
		{ID: "Standard_B2s", Provider: "azure", Family: "burstable", Series: "B", VCPUs: 2, MemoryGiB: 4.0, NetworkGbps: 1.28, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.0416, SpotPricePerHour: 0.0083, Tags: []string{"burstable", "ci"}},
		{ID: "Standard_B2ms", Provider: "azure", Family: "burstable", Series: "B", VCPUs: 2, MemoryGiB: 8.0, NetworkGbps: 1.92, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.0832, SpotPricePerHour: 0.0166, Tags: []string{"burstable", "ci"}},
		{ID: "Standard_B4ms", Provider: "azure", Family: "burstable", Series: "B", VCPUs: 4, MemoryGiB: 16.0, NetworkGbps: 3.84, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.1660, SpotPricePerHour: 0.0332, Tags: []string{"burstable", "ci"}},
		{ID: "Standard_B8ms", Provider: "azure", Family: "burstable", Series: "B", VCPUs: 8, MemoryGiB: 32.0, NetworkGbps: 7.68, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.3330, SpotPricePerHour: 0.0666, Tags: []string{"burstable", "ci"}},

		// General purpose D-series v5
		{ID: "Standard_D2s_v5", Provider: "azure", Family: "general-purpose", Series: "Dsv5", VCPUs: 2, MemoryGiB: 8.0, NetworkGbps: 12.5, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.0960, SpotPricePerHour: 0.0192, Tags: []string{"general", "intel", "ci", "standard"}},
		{ID: "Standard_D4s_v5", Provider: "azure", Family: "general-purpose", Series: "Dsv5", VCPUs: 4, MemoryGiB: 16.0, NetworkGbps: 12.5, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.1920, SpotPricePerHour: 0.0384, Tags: []string{"general", "intel", "ci", "standard"}},
		{ID: "Standard_D8s_v5", Provider: "azure", Family: "general-purpose", Series: "Dsv5", VCPUs: 8, MemoryGiB: 32.0, NetworkGbps: 12.5, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.3840, SpotPricePerHour: 0.0768, Tags: []string{"general", "intel", "ci", "standard"}},
		{ID: "Standard_D16s_v5", Provider: "azure", Family: "general-purpose", Series: "Dsv5", VCPUs: 16, MemoryGiB: 64.0, NetworkGbps: 12.5, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.7680, SpotPricePerHour: 0.1536, Tags: []string{"general", "intel", "ci", "standard"}},

		// ARM Dps v5 series
		{ID: "Standard_D2ps_v5", Provider: "azure", Family: "general-purpose", Series: "Dpsv5", VCPUs: 2, MemoryGiB: 8.0, NetworkGbps: 12.5, StorageType: "premium-ssd", Architecture: "arm64", OnDemandPricePerHour: 0.0768, SpotPricePerHour: 0.0154, Tags: []string{"general", "arm", "ampere", "ci"}},
		{ID: "Standard_D4ps_v5", Provider: "azure", Family: "general-purpose", Series: "Dpsv5", VCPUs: 4, MemoryGiB: 16.0, NetworkGbps: 12.5, StorageType: "premium-ssd", Architecture: "arm64", OnDemandPricePerHour: 0.1536, SpotPricePerHour: 0.0307, Tags: []string{"general", "arm", "ampere", "ci"}},

		// Compute optimized F-series v2
		{ID: "Standard_F2s_v2", Provider: "azure", Family: "compute-optimized", Series: "Fsv2", VCPUs: 2, MemoryGiB: 4.0, NetworkGbps: 0.875, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.0850, SpotPricePerHour: 0.0170, Tags: []string{"compute", "intel", "build", "ci"}},
		{ID: "Standard_F4s_v2", Provider: "azure", Family: "compute-optimized", Series: "Fsv2", VCPUs: 4, MemoryGiB: 8.0, NetworkGbps: 1.75, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.1700, SpotPricePerHour: 0.0340, Tags: []string{"compute", "intel", "build", "ci"}},
		{ID: "Standard_F8s_v2", Provider: "azure", Family: "compute-optimized", Series: "Fsv2", VCPUs: 8, MemoryGiB: 16.0, NetworkGbps: 3.0, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.3380, SpotPricePerHour: 0.0676, Tags: []string{"compute", "intel", "build", "ci"}},
		{ID: "Standard_F16s_v2", Provider: "azure", Family: "compute-optimized", Series: "Fsv2", VCPUs: 16, MemoryGiB: 32.0, NetworkGbps: 6.0, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.6770, SpotPricePerHour: 0.1354, Tags: []string{"compute", "intel", "build", "ci"}},

		// Memory optimized E-series v5
		{ID: "Standard_E2s_v5", Provider: "azure", Family: "memory-optimized", Series: "Esv5", VCPUs: 2, MemoryGiB: 16.0, NetworkGbps: 12.5, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.1260, SpotPricePerHour: 0.0252, Tags: []string{"memory", "intel", "ci"}},
		{ID: "Standard_E4s_v5", Provider: "azure", Family: "memory-optimized", Series: "Esv5", VCPUs: 4, MemoryGiB: 32.0, NetworkGbps: 12.5, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.2520, SpotPricePerHour: 0.0504, Tags: []string{"memory", "intel", "ci"}},

		// GPU NC T4 v3 series
		{ID: "Standard_NC4as_T4_v3", Provider: "azure", Family: "gpu", Series: "NCasT4v3", VCPUs: 4, MemoryGiB: 28.0, NetworkGbps: 8.0, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.5260, SpotPricePerHour: 0.1052, Tags: []string{"gpu", "nvidia", "t4", "ml", "ci"}},
		{ID: "Standard_NC8as_T4_v3", Provider: "azure", Family: "gpu", Series: "NCasT4v3", VCPUs: 8, MemoryGiB: 56.0, NetworkGbps: 8.0, StorageType: "premium-ssd", Architecture: "x86_64", OnDemandPricePerHour: 0.7520, SpotPricePerHour: 0.1504, Tags: []string{"gpu", "nvidia", "t4", "ml", "ci"}},
	}

	sort.Slice(machines, func(i, j int) bool {
		return machines[i].OnDemandPricePerHour < machines[j].OnDemandPricePerHour
	})

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(machines); err != nil {
		fmt.Fprintf(os.Stderr, "error encoding JSON: %v\n", err)
		os.Exit(1)
	}
}

// fetchAzurePrices fetches prices from Azure Retail Prices API
// This is a placeholder for full API integration
func fetchAzurePrices(region string) ([]AzurePriceItem, error) {
	baseURL := "https://prices.azure.com/api/retail/prices"
	filter := fmt.Sprintf("armRegionName eq '%s' and serviceName eq 'Virtual Machines' and priceType eq 'Consumption'", region)
	
	url := fmt.Sprintf("%s?$filter=%s", baseURL, strings.ReplaceAll(filter, " ", "%20"))
	
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	
	var priceResp AzurePriceResponse
	if err := json.Unmarshal(body, &priceResp); err != nil {
		return nil, err
	}
	
	return priceResp.Items, nil
}

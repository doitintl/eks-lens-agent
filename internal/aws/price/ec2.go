package price

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/doitintl/eks-lens-agent/internal/aws/global"
	"github.com/pkg/errors"
)

/*
Case-sensitive URL addresses for EC2 pricing:

Linux: https://b0.p.awsstatic.com/pricing/2.0/meteredUnitMaps/ec2/USD/current/ec2-ondemand-without-sec-sel/$region/Linux/index.json
Windows: https://b0.p.awsstatic.com/pricing/2.0/meteredUnitMaps/ec2/USD/current/ec2-ondemand-without-sec-sel/$region/Windows/index.json
RHEL: https://b0.p.awsstatic.com/pricing/2.0/meteredUnitMaps/ec2/USD/current/ec2-ondemand-without-sec-sel/$region/RHEL/index.json
SUSE: https://b0.p.awsstatic.com/pricing/2.0/meteredUnitMaps/ec2/USD/current/ec2-ondemand-without-sec-sel/$region/SUSE/index.json
Ubuntu Pro: https://b0.p.awsstatic.com/pricing/2.0/meteredUnitMaps/ec2/USD/current/ec2-ondemand-without-sec-sel/US%20East%20(Ohio)/Ubuntu%20Pro/index.json

where region is URL encoded long name of the region, e.g. us-east-2 is US%20East%20(Ohio)
*/

type Prices map[string]float64

var (
	regionOSPrices = map[string]Prices{}
)

type RegionExplorer interface {
	GetRegionMap(ctx context.Context) (map[string]global.Region, error)
}

func GetInstancePrice(ctx context.Context, explorer RegionExplorer, regionID, os, osImage, instanceType string) (float64, error) {
	// load regions map
	regions, err := explorer.GetRegionMap(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "loading regions map")
	}
	// get regionID long name
	region, ok := regions[regionID]
	if !ok {
		return 0, errors.Errorf("regionID %s not found", regionID)
	}

	// construct key = regionID/os
	key := fmt.Sprintf("%s/%s", regionID, os)
	// lazy load pricing for regionID and os
	if _, ok = regionOSPrices[key]; !ok {
		prices, err := loadEC2Pricing(region.LongName, getOSName(os, osImage))
		if err != nil {
			return 0, errors.Wrap(err, "loading EC2 pricing")
		}
		regionOSPrices[key] = prices
	}
	// get price for instance type
	price, ok := regionOSPrices[key][instanceType]
	if !ok {
		return 0, errors.Errorf("instance type %s not found", instanceType)
	}
	return price, nil
}

func getOSName(os, osImage string) string {
	const defaultOS = "Linux"
	switch os {
	case "linux":
		if strings.HasPrefix(osImage, "Red Hat") {
			return "RHEL"
		}
		if strings.HasPrefix(osImage, "SLES") {
			return "SUSE"
		}
		if strings.HasPrefix(osImage, "Ubuntu") {
			return "Ubuntu Pro"
		}
		return defaultOS
	case "windows":
		return "Windows"
	default:
		return defaultOS
	}
}

func loadEC2Pricing(regionLongName, os string) (Prices, error) {
	// ec2 pricing address
	const ec2PricingURL = "https://b0.p.awsstatic.com/pricing/2.0/meteredUnitMaps/ec2/USD/current/ec2-ondemand-without-sec-sel/%s/%s/index.json"
	// URL encode regionLongName name
	name := url.QueryEscape(regionLongName)
	// build URL
	address := fmt.Sprintf(ec2PricingURL, name, os)
	// load pricing using http client
	resp, err := http.Get(address) //nolint:gosec
	defer func() {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
	}()
	if err != nil {
		return nil, errors.Wrap(err, "loading EC2 pricing")
	}
	// parse pricing
	var pricing map[string]interface{}
	if err = json.NewDecoder(resp.Body).Decode(&pricing); err != nil {
		return nil, errors.Wrap(err, "parsing EC2 pricing")
	}
	// build map of instance type to price
	prices := make(map[string]float64)
	for _, r := range pricing["regions"].(map[string]interface{}) {
		for _, p := range r.(map[string]interface{}) {
			v := p.(map[string]interface{})
			instanceType := v["Instance Type"].(string)
			price := v["price"].(string)
			// convert price to float64
			prices[instanceType], err = strconv.ParseFloat(price, 64)
			if err != nil {
				return nil, errors.Wrap(err, "parsing EC2 price")
			}
		}
	}
	return prices, nil
}

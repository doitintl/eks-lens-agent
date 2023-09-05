package global

import (
	"context"
	"strings"
	"sync"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/pkg/errors"
)

type Region struct {
	ID       string
	LongName string
}

var (
	once      sync.Once
	regionMap map[string]Region
)

type AWSRegionExplorer struct{}

// GetRegionMap returns a map of region ID to Region
func (aws AWSRegionExplorer) GetRegionMap(ctx context.Context) (map[string]Region, error) {
	var err error
	// load the region map from SSM parameter store, do it only once
	once.Do(func() {
		regionMap, err = loadRegionMap(ctx)
	})
	return regionMap, err
}

// loadRegionMap lazy load the region map from SSM parameter store
func loadRegionMap(ctx context.Context) (map[string]Region, error) {
	// create a new Amazon SSM client
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "loading AWS config")
	}
	svc := ssm.NewFromConfig(cfg)
	// initialize the region map
	regionMap = make(map[string]Region)
	// get the region map from SSM parameter store
	// /aws/service/global-infrastructure/regions
	var nextToken *string
	for {
		// Request all regions, paginating the results if needed
		input := &ssm.GetParametersByPathInput{
			Path:      aws.String("/aws/service/global-infrastructure/regions"),
			NextToken: nextToken,
		}
		output, err := svc.GetParametersByPath(ctx, input)
		if err != nil {
			return nil, errors.Wrap(err, "getting region map from SSM parameter store")
		}

		// construct parameter names from the output
		names := make([]string, 0, len(output.Parameters))
		for _, param := range output.Parameters {
			region := (*param.Name)[strings.LastIndex(*param.Name, "/")+1:]
			names = append(names, "/aws/service/global-infrastructure/regions/"+region+"/longName")
		}

		paramsOutput, err := svc.GetParameters(ctx, &ssm.GetParametersInput{Names: names})
		if err != nil {
			return nil, errors.Wrap(err, "getting region longName from SSM parameter store")
		}

		for _, param := range paramsOutput.Parameters {
			// get the region ID from the parameter name two before the last slash
			tokens := strings.Split(*param.Name, "/")
			region := tokens[len(tokens)-2]
			regionMap[region] = Region{
				ID:       region,
				LongName: *param.Value,
			}
		}

		// if there are more regions, get the next page
		nextToken = output.NextToken
		if nextToken == nil {
			break
		}
	}
	return regionMap, nil
}

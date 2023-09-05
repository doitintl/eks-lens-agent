package price

import (
	"context"
	"testing"
)

func Test_loadEC2Pricing(t *testing.T) {
	type args struct {
		region string
		os     string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			name: "t4g.xlarge Linux in US East (Ohio)",
			args: args{
				region: "US East (Ohio)",
				os:     "Linux",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := loadEC2Pricing(tt.args.region, tt.args.os)
			if (err != nil) != tt.wantErr {
				t.Errorf("loadEC2Pricing() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if price, ok := got["t4g.xlarge"]; !ok || price == 0 {
				t.Errorf("loadEC2Pricing() = %v", got)
			}
			t.Log("t4g.xlarge price:", got["t4g.xlarge"])
		})
	}
}

func TestGetInstancePrice(t *testing.T) {
	type args struct {
		ctx          context.Context
		regionID     string
		os           string
		osImage      string
		instanceType string
	}
	tests := []struct {
		name    string
		args    args
		wantErr bool
	}{
		{
			// t4g.xlarge Linux in US East (Ohio)
			name: "t4g.xlarge Linux in US East (Ohio)",
			args: args{
				ctx:          context.Background(),
				regionID:     "us-east-2",
				os:           "linux",
				osImage:      "Amazon Linux 2",
				instanceType: "t4g.xlarge",
			},
		},
		{
			// m5a.4xlarge Windows in US West (Oregon)
			name: "m5a.4xlarge Windows in US West (Oregon)",
			args: args{
				ctx:          context.Background(),
				regionID:     "us-west-2",
				os:           "windows",
				osImage:      "Windows Server 2019 Base",
				instanceType: "m5a.4xlarge",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := GetInstancePrice(tt.args.ctx, tt.args.regionID, tt.args.os, tt.args.osImage, tt.args.instanceType)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetInstancePrice() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got == 0 {
				t.Errorf("GetInstancePrice() = %v", got)
			}
			t.Log(tt.args.instanceType, " price:", got)
		})
	}
}

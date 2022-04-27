package mockroute53

import (
	"fmt"
	"reflect"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
)

func TestListHostedZones(t *testing.T) {
	tests := []struct {
		Name         string
		TestClient   *MockRoute53Client
		ExpectedLHZO *route53.ListHostedZonesOutput
		ExpectError  bool
	}{
		{
			Name: "returns mock hosted zone output",
			TestClient: &MockRoute53Client{
				ZoneCount: 150,
			},
			ExpectedLHZO: &route53.ListHostedZonesOutput{
				HostedZones: func() []*route53.HostedZone {
					lhzo := []*route53.HostedZone{}
					for i := 0; i <= 99; i++ {
						lhzo = append(lhzo, &route53.HostedZone{
							CallerReference: aws.String(fmt.Sprintf("zone%d", i)),
							Id:              aws.String(fmt.Sprintf("id%d", i)),
							Name:            aws.String(fmt.Sprintf("name%d.", i)),
						})
					}
					return lhzo
				}(),
				IsTruncated: aws.Bool(true),
				NextMarker:  aws.String("id100"),
			},
			ExpectError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			lhzo, err := test.TestClient.ListHostedZones(&route53.ListHostedZonesInput{})
			if test.ExpectError == (err == nil) {
				t.Errorf("ListHostedZones() %s: ExpectError: %t, actual error: %s\n", test.Name, test.ExpectError, err)
			}

			if !reflect.DeepEqual(lhzo, test.ExpectedLHZO) {
				t.Errorf("ListHostedZones() %s: expected %v, got %v\n", test.Name, test.ExpectedLHZO, lhzo)
			}
		})
	}
}

func TestGetHostedZone(t *testing.T) {
	tests := []struct {
		Name           string
		Client         *MockRoute53Client
		Input          *route53.GetHostedZoneInput
		ExpectedOutput *route53.GetHostedZoneOutput
		ExpectError    bool
	}{
		{
			Name: "returns the zone",
			Client: &MockRoute53Client{
				ZoneCount: 1,
			},
			Input: &route53.GetHostedZoneInput{
				Id: aws.String("id0"),
			},
			ExpectedOutput: &route53.GetHostedZoneOutput{
				DelegationSet: &route53.DelegationSet{
					NameServers: []*string{},
				},
				HostedZone: &route53.HostedZone{
					CallerReference: aws.String("zone0"),
					Id:              aws.String("id0"),
					Name:            aws.String("name0."),
					Config: &route53.HostedZoneConfig{
						PrivateZone: aws.Bool(false),
					},
				},
			},
			ExpectError: false,
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			actualOutput, err := test.Client.GetHostedZone(test.Input)
			if test.ExpectError == (err == nil) {
				t.Errorf("GetHostedZone() %s: ExpectError: %t, actual error: %s\n", test.Name, test.ExpectError, err)
			}

			if !reflect.DeepEqual(actualOutput, test.ExpectedOutput) {
				t.Errorf("GetHostedZone() %s: expected %v, got %v\n", test.Name, test.ExpectedOutput, actualOutput)
			}
		})
	}
}

func TestChangeResourceRecordSets(t *testing.T) {
	tests := []struct {
		Name        string
		Client      *MockRoute53Client
		Input       *route53.ChangeResourceRecordSetsInput
		ExpectError bool
	}{
		{
			Name: "mocks a change",
			Client: &MockRoute53Client{
				ZoneCount: 1,
			},
			Input: &route53.ChangeResourceRecordSetsInput{
				ChangeBatch: &route53.ChangeBatch{
					Changes: []*route53.Change{
						{
							Action: aws.String(route53.ChangeActionUpsert),
							ResourceRecordSet: &route53.ResourceRecordSet{
								Name: aws.String("name0"),
								ResourceRecords: []*route53.ResourceRecord{
									{
										Value: aws.String("\"test_challenge_token\""),
									},
								},
								TTL:  aws.Int64(15),
								Type: aws.String(route53.RRTypeTxt),
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			actualOutput, err := test.Client.ChangeResourceRecordSets(test.Input)
			if test.ExpectError == (err == nil) {
				t.Errorf("ChangeResourceRecordSets() %s: ExpectError: %t, actual error: %s\n", test.Name, test.ExpectError, err)
			}

			// we don't really use the returned output from ChangeResourceRecordSets()
			if reflect.TypeOf(actualOutput).String() != "*route53.ChangeResourceRecordSetsOutput" {
				t.Errorf("ChangeResourceRecordSets() %s: expected result to be type ChangeResourceRecordSetsOutput, got type %s\n", test.Name, reflect.TypeOf(actualOutput).String())
			}
		})
	}
}

func TestListResourceRecordSets(t *testing.T) {
	tests := []struct {
		Name           string
		Client         *MockRoute53Client
		Input          *route53.ListResourceRecordSetsInput
		ExpectedOutput *route53.ListResourceRecordSetsOutput
		ExpectError    bool
	}{
		{
			Name: "mocks listing resource record sets",
			Client: &MockRoute53Client{
				ZoneCount: 1,
			},
			Input: &route53.ListResourceRecordSetsInput{
				HostedZoneId: aws.String("zone0"),
			},
			ExpectedOutput: &route53.ListResourceRecordSetsOutput{
				ResourceRecordSets: []*route53.ResourceRecordSet{
					{
						Name: aws.String("_acme-challenge.api.gibberish.goes.here."),
						Type: aws.String(route53.RRTypeTxt),
						ResourceRecords: []*route53.ResourceRecord{
							{
								Value: aws.String("test"),
							},
						},
					},
				},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.Name, func(t *testing.T) {
			actualOutput, err := test.Client.ListResourceRecordSets(test.Input)
			if test.ExpectError == (err == nil) {
				t.Errorf("ListResourceRecordSets() %s: ExpectError: %t, actual error: %s\n", test.Name, test.ExpectError, err)
			}

			if !reflect.DeepEqual(actualOutput, test.ExpectedOutput) {
				t.Errorf("ListResourceRecordSets() %s: expected %v, got %v\n", test.Name, test.ExpectedOutput, actualOutput)
			}
		})
	}
}

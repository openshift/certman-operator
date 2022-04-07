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
							Name:            aws.String(fmt.Sprintf("name%d", i)),
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

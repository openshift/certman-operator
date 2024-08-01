package mockroute53

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
)

type MockRoute53Client struct {
	route53iface.Route53API
	ZoneCount int
}

func (m *MockRoute53Client) GetFedrampHostedZoneIDPath(fedrampHostedZoneID string) (string, error) {
	zone := &route53.GetHostedZoneOutput{
		HostedZone: &route53.HostedZone{
			Id: &fedrampHostedZoneID,
		},
	}
	return *zone.HostedZone.Id, nil
}

func (m *MockRoute53Client) ListHostedZones(lhzi *route53.ListHostedZonesInput) (*route53.ListHostedZonesOutput, error) {
	hostedZones := []*route53.HostedZone{}

	// figure out the start zone for the request
	var startI int
	if lhzi.Marker == nil {
		startI = 0
	} else {
		startI, _ = strconv.Atoi(strings.TrimLeft(*lhzi.Marker, "id"))
	}

	var maxItems int
	if lhzi.MaxItems == nil {
		maxItems = 100
	} else {
		maxItems, _ = strconv.Atoi(*lhzi.MaxItems)
	}

	// figure out the end zone for the request
	var endI int
	if startI+maxItems < m.ZoneCount {
		endI = startI + maxItems
	} else {
		endI = m.ZoneCount
	}

	// generate fake zones between the start marker and either the maxitems or the end of the zonecount
	var nextMarker string
	for i := startI; i < endI; i++ {
		callerRef := fmt.Sprintf("zone%d", i)
		id := fmt.Sprintf("id%d", i)
		name := fmt.Sprintf("name%d.", i)

		hz := route53.HostedZone{
			CallerReference: &callerRef,
			Id:              &id,
			Name:            &name,
		}

		hostedZones = append(hostedZones, &hz)

		nextMarker = fmt.Sprintf("id%d", i+1)
	}

	isTruncated := endI < m.ZoneCount

	output := &route53.ListHostedZonesOutput{
		HostedZones: hostedZones,
		IsTruncated: &isTruncated,
		Marker:      lhzi.Marker,
		MaxItems:    lhzi.MaxItems,
	}

	if isTruncated {
		output.NextMarker = &nextMarker
	}

	return output, nil
}

func (c *MockRoute53Client) GetHostedZone(input *route53.GetHostedZoneInput) (output *route53.GetHostedZoneOutput, err error) {
	idNumber := strings.Split(*input.Id, "id")[1]
	output = &route53.GetHostedZoneOutput{
		DelegationSet: &route53.DelegationSet{
			NameServers: []*string{},
		},
		HostedZone: &route53.HostedZone{
			CallerReference: aws.String(fmt.Sprintf("zone%s", idNumber)),
			Id:              input.Id,
			Name:            aws.String(fmt.Sprintf("name%s.", idNumber)),
			Config: &route53.HostedZoneConfig{
				PrivateZone: aws.Bool(false),
			},
		},
	}
	return
}

func (c *MockRoute53Client) ChangeResourceRecordSets(input *route53.ChangeResourceRecordSetsInput) (output *route53.ChangeResourceRecordSetsOutput, err error) {
	output = &route53.ChangeResourceRecordSetsOutput{
		ChangeInfo: &route53.ChangeInfo{
			Id:          aws.String("mockchangeid"),
			Status:      aws.String("PENDING"),
			SubmittedAt: aws.Time(time.Now()),
		},
	}
	return
}

func (c *MockRoute53Client) ListResourceRecordSets(input *route53.ListResourceRecordSetsInput) (output *route53.ListResourceRecordSetsOutput, err error) {
	output = &route53.ListResourceRecordSetsOutput{
		ResourceRecordSets: []*route53.ResourceRecordSet{
			{},
		},
	}
	output.ResourceRecordSets[0].Name = aws.String("_acme-challenge.api.gibberish.goes.here.")
	output.ResourceRecordSets[0].Type = aws.String(route53.RRTypeTxt)

	for i := 0; i < c.ZoneCount; i++ {
		output.ResourceRecordSets[0].ResourceRecords = append(output.ResourceRecordSets[0].ResourceRecords, &route53.ResourceRecord{
			Value: aws.String("test"),
		})
	}

	return
}

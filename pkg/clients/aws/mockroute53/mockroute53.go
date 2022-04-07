package mockroute53

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/aws/aws-sdk-go/service/route53/route53iface"
)

type MockRoute53Client struct {
	route53iface.Route53API
	ZoneCount int
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
		name := fmt.Sprintf("name%d", i)

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

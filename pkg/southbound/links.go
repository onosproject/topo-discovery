// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package southbound

import (
	"github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	"github.com/onosproject/onos-net-lib/pkg/gnmiutils"
	"github.com/onosproject/onos-net-lib/pkg/stratum"
	"github.com/openconfig/gnmi/proto/gnmi"
	"strconv"
)

// Link holds information about an ingress link
type Link struct {
	IngressDevice string
	IngressPort   uint32
	EgressDevice  string
	EgressPort    uint32
}

// LinkReport provides results of a link query against the link local agent
type LinkReport struct {
	ID      topo.ID
	AgentID string
	Links   map[uint32]*Link
}

// GetIngressLinks returns a map of link descriptors obtained via gNMI get request on state/link[port=...] query
func GetIngressLinks(object *topo.Object) (*LinkReport, error) {
	report := &LinkReport{ID: object.ID, Links: make(map[uint32]*Link)}
	localAgents := &topo.LocalAgents{}
	if err := object.GetAspect(localAgents); err != nil {
		log.Warnf("Object %s doesn't have onos.topo.LocalAgents aspect", object.ID)
		return nil, err
	}

	// Connect to the device's link local agent using gNMI
	device, err := stratum.NewGNMI(string(object.ID), localAgents.LinkAgentEndpoint, true)
	if err != nil {
		log.Warnf("Unable to connect to Stratum link local agent gNMI %s: %+v", object.ID, err)
		return nil, err
	}
	defer device.Disconnect()

	// Get the agent ID first
	if report.AgentID, err = getAgentID(device); err != nil {
		return nil, err
	}

	resp, err := device.Client.Get(device.Context, &gnmi.GetRequest{
		Path: []*gnmi.Path{gnmiutils.ToPath("state/link[port=...]")},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Notification) == 0 {
		return nil, errors.NewInvalid("no link data received")
	}

	log.Debugf("%s: Got links: %+v", object.ID, resp.Notification)

	for _, update := range resp.Notification[0].Update {
		port, err1 := strconv.ParseInt(update.Path.Elem[1].Key["port"], 10, 32)
		if err1 != nil {
			log.Warnf("Key 'port' is not a number: %+v", err)
			continue
		}
		link := getLink(report.Links, report.AgentID, uint32(port))
		last := len(update.Path.Elem) - 1
		switch update.Path.Elem[last].Name {
		case "egress-port":
			link.EgressPort = uint32(update.Val.GetIntVal())
		case "egress-device":
			link.EgressDevice = update.Val.GetStringVal()
		}
	}
	return report, nil
}

func getAgentID(device *stratum.GNMI) (string, error) {
	resp, err := device.Client.Get(device.Context, &gnmi.GetRequest{
		Path: []*gnmi.Path{gnmiutils.ToPath("state/agent-id")},
	})
	if err != nil {
		return "", err
	}
	if len(resp.Notification) == 0 || len(resp.Notification[0].Update) == 0 {
		return "", errors.NewInvalid("agent-id not received")
	}
	return resp.Notification[0].Update[0].Val.GetStringVal(), nil
}

func getLink(links map[uint32]*Link, agentID string, port uint32) *Link {
	link, ok := links[port]
	if !ok {
		link = &Link{IngressDevice: agentID, IngressPort: port}
		links[port] = link
	}
	return link
}

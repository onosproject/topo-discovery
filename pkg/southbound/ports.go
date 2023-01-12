// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

// Package southbound contains utilities to interact with Stratum devices via gNMI and P4Runtime
package southbound

import (
	"github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-net-lib/pkg/gnmiutils"
	"github.com/onosproject/onos-net-lib/pkg/stratum"
	"github.com/openconfig/gnmi/proto/gnmi"
)

var log = logging.GetLogger("southbound")

// GetPorts returns a map of port descriptors obtained via gNMI get request on /interfaces/interface[name=...] query
func GetPorts(object *topo.Object) (map[string]*topo.Port, error) {
	// Connect to the device using gNMI
	device, err := stratum.NewStratumGNMI(object, true)
	if err != nil {
		log.Warnf("Unable to connect to Stratum device gNMI %s: %+v", object.ID, err)
		return nil, err
	}
	defer device.Disconnect()

	resp, err := device.Client.Get(device.Context, &gnmi.GetRequest{
		Path: []*gnmi.Path{
			gnmiutils.ToPath("interfaces/interface[name=...]/state"),
			gnmiutils.ToPath("interfaces/interface[name=...]/config"),
			gnmiutils.ToPath("interfaces/interface[name=...]/ethernet/config"),
		},
	})
	if err != nil {
		return nil, err
	}
	if len(resp.Notification) == 0 {
		return nil, errors.NewInvalid("no port data received")
	}

	ports := make(map[string]*topo.Port)
	for _, notification := range resp.Notification {
		for _, update := range notification.Update {
			port := getPort(ports, update.Path.Elem[1].Key["name"])
			last := len(update.Path.Elem) - 1
			switch update.Path.Elem[last].Name {
			case "ifindex":
				port.Index = uint32(update.Val.GetUintVal())
			case "id":
				port.Number = uint32(update.Val.GetUintVal())
			case "oper-status":
				port.Status = update.Val.GetStringVal()
			case "last-change":
				port.LastChange = update.Val.GetUintVal()
			case "port-speed":
				port.Speed = update.Val.GetStringVal()
			case "enabled":
				port.Enabled = update.Val.GetBoolVal()
			}
		}
	}
	return ports, nil
}

func getPort(ports map[string]*topo.Port, id string) *topo.Port {
	port, ok := ports[id]
	if !ok {
		port = &topo.Port{DisplayName: id}
		ports[id] = port
	}
	return port
}

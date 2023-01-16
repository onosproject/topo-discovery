// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

// Package southbound contains utilities to interact with Stratum devices via gNMI and P4Runtime
package southbound

import (
	"context"
	"fmt"
	"github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-net-lib/pkg/gnmiutils"
	"github.com/onosproject/onos-net-lib/pkg/stratum"
	"github.com/openconfig/gnmi/proto/gnmi"
	"io"
	"sync"
)

var log = logging.GetLogger("southbound")

// PortStatusListener is an abstraction of an entity capable of handling port status updates
type PortStatusListener interface {
	HandlePortStatus(object *topo.Object, port *topo.Port)
}

// PortDiscovery is an abstraction of an entity capable of discovering device ports
type PortDiscovery interface {
	GetPorts(object *topo.Object, listener PortStatusListener) (map[string]*topo.Port, error)
}

// Implementation of PortDiscovery using gNMI against Stratum device agent
type gNMIPortDiscovery struct {
	PortDiscovery
	lock           sync.RWMutex
	deviceContexts map[topo.ID]*deviceContext
}

// Stratum device gNMI port discovery context
type deviceContext struct {
	object    *topo.Object
	device    *stratum.GNMI
	listener  PortStatusListener
	ctx       context.Context
	ctxCancel context.CancelFunc
	ports     map[string]*topo.Port
}

// NewGNMIPortDiscovery returns new port discovery based on gNMI
func NewGNMIPortDiscovery() PortDiscovery {
	return &gNMIPortDiscovery{deviceContexts: make(map[topo.ID]*deviceContext, 4)}
}

// GetPorts returns a map of port descriptors obtained via gNMI get request on /interfaces/interface[name=...] query
func (pd *gNMIPortDiscovery) GetPorts(object *topo.Object, listener PortStatusListener) (map[string]*topo.Port, error) {
	// Get the gNMI device context; get existing one or create one
	dc, err := pd.getDeviceContext(object, listener)
	if err != nil {
		return nil, err
	}

	resp, err := dc.device.Client.Get(dc.device.Context, &gnmi.GetRequest{
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

	// Trigger monitor restart only if port numbers change; record new ports map every time though
	restart := len(dc.ports) != len(ports)
	dc.ports = ports

	// Once ports are discovered kick off a port-status monitor, if necessary
	dc.startMonitor(restart)

	return ports, nil
}

func (pd *gNMIPortDiscovery) getDeviceContext(object *topo.Object, listener PortStatusListener) (*deviceContext, error) {
	pd.lock.Lock()
	defer pd.lock.Unlock()
	dc, ok := pd.deviceContexts[object.ID]
	if !ok {
		dc = &deviceContext{object: object, listener: listener}
		pd.deviceContexts[object.ID] = dc
	}

	// If we haven't established a device context yet, do so.
	if dc.device == nil {
		// Connect to the device using gNMI
		var err error
		dc.device, err = stratum.NewStratumGNMI(object, true)
		if err != nil {
			log.Warnf("Unable to connect to Stratum device gNMI %s: %+v", object.ID, err)
			return nil, err
		}
	}
	return dc, nil
}

func getPort(ports map[string]*topo.Port, id string) *topo.Port {
	port, ok := ports[id]
	if !ok {
		port = &topo.Port{DisplayName: id}
		ports[id] = port
	}
	return port
}

// Starts the port monitor if not already started or if restart is requested
func (dc *deviceContext) startMonitor(restart bool) {
	if dc.ctxCancel == nil || restart {
		dc.stopMonitor()
		log.Infof("Starting port status monitor...")
		go dc.monitorPortStatus()
	}
}

// Stops the port monitor
func (dc *deviceContext) stopMonitor() {
	if dc.ctxCancel != nil {
		log.Infof("Stopping port status monitor...")
		dc.ctxCancel()
		dc.ctxCancel = nil
	}
}

// Issues subscribe request for port state updates and monitors the stream for update notifications
func (dc *deviceContext) monitorPortStatus() {
	log.Infof("Port status monitor started")
	dc.ctx, dc.ctxCancel = context.WithCancel(context.Background())
	stream, err := dc.device.Client.Subscribe(dc.ctx)
	if err != nil {
		log.Warn("Unable to subscribe for port state updates: %+v", err)
		return
	}

	subscriptions := make([]*gnmi.Subscription, 0, len(dc.ports))
	for key := range dc.ports {
		subscriptions = append(subscriptions, &gnmi.Subscription{
			Path: gnmiutils.ToPath(fmt.Sprintf("interfaces/interface[name=%s]/state/oper-state", key)),
		})
	}
	if err = stream.Send(&gnmi.SubscribeRequest{
		Request: &gnmi.SubscribeRequest_Subscribe{
			Subscribe: &gnmi.SubscriptionList{Subscription: subscriptions},
		}}); err != nil {
		log.Warn("Unable to send subscription request for port state updates: %+v", err)
		return
	}

	for {
		resp, err := stream.Recv()
		if err != nil {
			if err != io.EOF {
				log.Warn("Unable to read subscription response for port state updates: %+v", err)
			}
			log.Infof("Port status monitor stopped")
			return
		}
		log.Infof("Got port status update %+v", resp.GetUpdate())
		if resp.GetUpdate() != nil && len(resp.GetUpdate().Update) > 0 {
			for _, update := range resp.GetUpdate().Update {
				if update.Path.Elem[(len(update.Path.Elem)-1)].Name == "oper-status" {
					portName := update.Path.Elem[1].Key["name"]
					port := getPort(dc.ports, portName)
					port.Status = update.Val.GetStringVal()
					dc.listener.HandlePortStatus(dc.object, port)
				}
			}
		}
	}
}

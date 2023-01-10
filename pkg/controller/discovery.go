// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package controller

import topoapi "github.com/onosproject/onos-api/go/onos/topo"

func (c *Controller) runInitialDiscoverySweep() {
	log.Infof("Running initial discovery sweep...")
	c.setState(Initialized)
	log.Infof("Initialized")
}

// Returns filters for matching objects on realm label, entity type and with GNMIServer aspect.
func queryFilter(realmLabel string, realmValue string) *topoapi.Filters {
	return &topoapi.Filters{
		LabelFilters: []*topoapi.Filter{{
			Filter: &topoapi.Filter_Equal_{Equal_: &topoapi.EqualFilter{Value: realmValue}},
			Key:    realmLabel,
		}},
		ObjectTypes: []topoapi.Object_Type{topoapi.Object_ENTITY},
		WithAspects: []string{"onos.topo.GNMIServer"},
	}
}

// Setup watch for updates using onos-topo API
func (c *Controller) prepareForMonitoring() {
	filter := queryFilter(c.realmLabel, c.realmValue)
	log.Infof("Starting to watch onos-topo via %+v", filter)
	stream, err := c.topoClient.Watch(c.ctx, &topoapi.WatchRequest{Filters: filter})
	if err != nil {
		log.Warnf("Unable to start onos-topo watch: %+v", err)
		c.setState(Disconnected)
	} else {
		go func() {
			for c.getState() == Monitoring {
				resp, err := stream.Recv()
				if err == nil && isRelevant(resp.Event) {
					c.queue <- &resp.Event.Object
				} else if err != nil {
					log.Warnf("Watch stream has been stopped: %+v", err)
					c.setStateIf(Monitoring, Disconnected)
				}
			}
		}()
		c.setState(Monitoring)
	}
}

// Returns true if the object is relevant to the controller
func isRelevant(event topoapi.Event) bool {
	return event.Type != topoapi.EventType_REMOVED
}

func (c *Controller) monitorTopologyChanges() {
	log.Infof("Monitoring topology changes...")
}

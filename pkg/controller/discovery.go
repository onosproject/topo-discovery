// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	"github.com/onosproject/onos-api/go/onos/topo"
	"io"
	"time"
)

func (c *Controller) runInitialDiscoverySweep() {
	for c.getState() == Connected {
		if err := c.runFullDiscoverySweep(); err == nil {
			c.setState(Initialized)
		} else {
			log.Warnf("Unable to query onos-topo: %+v", err)
			c.pauseIf(Disconnected, connectionRetryPause)
		}
	}
}

// Runs discovery sweep for all objects in our realm
func (c *Controller) runFullDiscoverySweep() error {
	log.Info("Starting full discovery sweep...")
	filter := queryFilter(c.realmLabel, c.realmValue)
	if entities, err := c.topoClient.Query(c.ctx, &topo.QueryRequest{Filters: filter}); err == nil {
		for c.getState() != Stopped {
			if entity, err := entities.Recv(); err == nil {
				c.queue <- entity.Object
			} else {
				if err == io.EOF {
					log.Info("Completed full discovery sweep")
					return nil
				}
				log.Warnf("Unable to read query response: %+v", err)
				return err
			}
		}
	} else {
		return err
	}
	return nil
}

// Returns filters for matching objects on realm label, entity type and with GNMIServer aspect.
func queryFilter(realmLabel string, realmValue string) *topo.Filters {
	return &topo.Filters{
		LabelFilters: []*topo.Filter{{
			Filter: &topo.Filter_Equal_{Equal_: &topo.EqualFilter{Value: realmValue}},
			Key:    realmLabel,
		}},
		ObjectTypes: []topo.Object_Type{topo.Object_ENTITY},
		WithAspects: []string{"onos.topo.StratumAgents"},
	}
}

// Setup watch for updates using onos-topo API
func (c *Controller) prepareForMonitoring() {
	filter := queryFilter(c.realmLabel, c.realmValue)
	log.Infof("Starting to watch onos-topo via %+v", filter)
	stream, err := c.topoClient.Watch(c.ctx, &topo.WatchRequest{Filters: filter})
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
func isRelevant(event topo.Event) bool {
	return event.Type != topo.EventType_REMOVED
}

func (c *Controller) monitorTopologyChanges() {
	tPeriodic := time.NewTicker(30 * time.Second)
	tCheckState := time.NewTicker(2 * time.Second)

	for c.getState() == Monitoring {
		select {
		// Periodically run a full discovery sweep
		case <-tPeriodic.C:
			_ = c.runFullDiscoverySweep()

		// Periodically pop-out to check state
		case <-tCheckState.C:
		}
	}
}

// Discovery worker
func (c *Controller) discover(workerID int) {
	for object := range c.queue {
		c.lock.Lock()

		// Is this object being worked on already?
		_, busy := c.workingOn[object.ID]
		if !busy {
			// If not, mark it as being worked on.
			c.workingOn[object.ID] = object
		}
		c.lock.Unlock()
		if !busy {
			log.Infof("%d: Working on %s", workerID, object.ID)
			c.portReconciler.DiscoverPorts(object)
			c.linkReconciler.DiscoverLinks(object)
			// c.discoverAtachedHosts(object)
			log.Infof("%d: Finished work on %s", workerID, object.ID)

			// We're done working on this object
			c.lock.Lock()
			delete(c.workingOn, object.ID)
			c.lock.Unlock()
		}
	}
}

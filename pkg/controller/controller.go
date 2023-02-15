// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

// Package controller implements the topology discovery controller
package controller

import (
	"context"
	"github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-net-lib/pkg/realm"
	"google.golang.org/grpc"
	"sync"
	"time"
)

var log = logging.GetLogger("controller")

// State represents the various states of controller lifecycle
type State int

const (
	// Disconnected represents the default/initial state
	Disconnected State = iota
	// Connected represents state where connection to onos-topo has been established
	Connected
	// Initialized represents state after the initial reconciliation pass has completed
	Initialized
	// Monitoring represents state of monitoring topology changes
	Monitoring
	// Stopped represents state where the controller has been issued a stop command
	Stopped
)

const (
	connectionRetryPause = 5 * time.Second

	queueDepth  = 128
	workerCount = 16
)

// Controller drives the topology discovery control logic
type Controller struct {
	realmOptions *realm.Options
	state        State

	lock sync.RWMutex

	topoAddress string
	topoOpts    []grpc.DialOption
	conn        *grpc.ClientConn
	topoClient  topo.TopoClient
	ctx         context.Context
	ctxCancel   context.CancelFunc
	queue       chan *topo.Object
	workingOn   map[topo.ID]*topo.Object

	portReconciler *PortReconciler
	linkReconciler *LinkReconciler
}

// NewController creates a new topology discovery controller
func NewController(realmOptions *realm.Options, topoAddress string, topoOpts ...grpc.DialOption) *Controller {
	opts := append(topoOpts, grpc.WithBlock())
	return &Controller{
		realmOptions: realmOptions,
		topoAddress:  topoAddress,
		topoOpts:     opts,
		workingOn:    make(map[topo.ID]*topo.Object),
	}
}

// Start starts the controller
func (c *Controller) Start() {
	log.Infof("Starting...")

	// Crate discovery job queue and workers
	c.queue = make(chan *topo.Object, queueDepth)
	for i := 0; i < workerCount; i++ {
		go c.discover(i)
	}

	go c.run()
}

// Stop stops the controller
func (c *Controller) Stop() {
	log.Infof("Stopping...")
	c.setState(Stopped)
	close(c.queue)
}

// Get the current operational state
func (c *Controller) getState() State {
	c.lock.RLock()
	defer c.lock.RUnlock()
	state := c.state
	return state
}

// Change state to the new state
func (c *Controller) setState(state State) {
	c.lock.Lock()
	defer c.lock.Unlock()
	c.state = state
}

// Change state to the new state, but only if in the given condition state
func (c *Controller) setStateIf(condition State, state State) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.state == condition {
		c.state = state
	}
}

// Pause for the specified duration, but only if in the given condition state
func (c *Controller) pauseIf(condition State, pause time.Duration) {
	if c.getState() == condition {
		time.Sleep(pause)
	}
}

// Runs the main controller event loop
func (c *Controller) run() {
	log.Infof("Started")
	for state := c.getState(); state != Stopped; state = c.getState() {
		switch state {
		case Disconnected:
			c.waitForTopoConnection()
		case Connected:
			c.runInitialDiscoverySweep()
		case Initialized:
			c.prepareForMonitoring()
		case Monitoring:
			c.monitorTopologyChanges()
		}
	}
	log.Infof("Stopped")
}

// Handles processing for Disconnected state by attempting to establish connection to onos-topo
func (c *Controller) waitForTopoConnection() {
	log.Infof("Connecting to onos-topo at %s...", c.topoAddress)
	for c.getState() == Disconnected {
		if conn, err := grpc.DialContext(context.Background(), c.topoAddress, c.topoOpts...); err == nil {
			c.conn = conn
			c.topoClient = topo.CreateTopoClient(conn)
			c.ctx, c.ctxCancel = context.WithCancel(context.Background())
			c.portReconciler = NewPortReconciler(c.ctx, c.topoClient)
			c.linkReconciler = NewLinkReconciler(c.ctx, c.topoClient)
			c.setState(Connected)
			log.Infof("Connected")
		} else {
			log.Warnf("Unable to connect to onos-topo: %+v", err)
			c.pauseIf(Disconnected, connectionRetryPause)
		}
	}
}

// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

// Package manager contains the topology discovery manager coordinating lifecycle of the NB API and SB controller
package manager

import (
	"github.com/onosproject/onos-lib-go/pkg/certs"
	"github.com/onosproject/onos-lib-go/pkg/cli"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-lib-go/pkg/northbound"
	"github.com/onosproject/topo-discovery/pkg/controller"
	nb "github.com/onosproject/topo-discovery/pkg/northbound"
)

var log = logging.GetLogger("manager")

// Config is a manager configuration
type Config struct {
	RealmLabel   string
	RealmValue   string
	TopoAddress  string
	ServiceFlags *cli.ServiceEndpointFlags
}

// Manager single point of entry for the topology discovery
type Manager struct {
	cli.Daemon
	Config     Config
	controller *controller.Controller
}

// NewManager initializes the application manager
func NewManager(cfg Config) *Manager {
	log.Infof("Creating manager")
	return &Manager{Config: cfg}
}

// Start initializes and starts the manager.
func (m *Manager) Start() error {
	log.Info("Starting Manager")

	// Initialize and start the configuration discovery controller
	opts, err := certs.HandleCertPaths(m.Config.ServiceFlags.CAPath, m.Config.ServiceFlags.KeyPath, m.Config.ServiceFlags.CertPath, true)
	if err != nil {
		return err
	}

	m.controller = controller.NewController(m.Config.RealmLabel, m.Config.RealmValue, m.Config.TopoAddress, opts...)
	m.controller.Start()

	// Start NB server
	s := northbound.NewServer(cli.ServerConfigFromFlags(m.Config.ServiceFlags, northbound.SecurityConfig{}))
	s.AddService(logging.Service{})
	s.AddService(nb.NewService(m.controller))
	return s.StartInBackground()
}

// Stop stops the manager
func (m *Manager) Stop() {
	log.Info("Stopping Manager")
	m.controller.Stop()
}

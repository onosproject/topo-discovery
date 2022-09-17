// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package manager

import (
	"github.com/onosproject/onos-lib-go/pkg/certs"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-lib-go/pkg/northbound"
	"github.com/onosproject/onos-p4-sdk/pkg/p4rt/appsdk"
	"github.com/onosproject/topo-discovery/pkg/controller/link"
	"github.com/onosproject/topo-discovery/pkg/controller/logicalinterface"
	"github.com/onosproject/topo-discovery/pkg/controller/logicalinterfacerelation"
	"github.com/onosproject/topo-discovery/pkg/store/topo"
)

var log = logging.GetLogger()

// Config is app manager
type Config struct {
	CAPath      string
	KeyPath     string
	CertPath    string
	TopoAddress string
	GRPCPort    int
	P4Plugins   []string
}

// Manager single point of entry for the device-provisioner application
type Manager struct {
	Config Config
}

// NewManager initializes the application manager
func NewManager(cfg Config) *Manager {
	log.Info("Creating application manager")
	mgr := Manager{
		Config: cfg,
	}
	return &mgr
}

// Run runs application manager
func (m *Manager) Run() {
	log.Info("Starting application Manager")

	if err := m.start(); err != nil {
		log.Fatal("Unable to run Manager", "error", err)
	}
}

func (m *Manager) start() error {

	opts, err := certs.HandleCertPaths(m.Config.CAPath, m.Config.KeyPath, m.Config.CertPath, true)
	if err != nil {
		return err
	}

	appsdk.StartController(appsdk.Config{
		CAPath:      m.Config.CAPath,
		CertPath:    m.Config.CertPath,
		KeyPath:     m.Config.KeyPath,
		TopoAddress: m.Config.TopoAddress,
	})

	// Create new topo store
	topoStore, err := topo.NewStore(m.Config.TopoAddress, opts...)
	if err != nil {
		return err
	}

	err = m.startInterfaceController(topoStore)
	if err != nil {
		return err
	}

	err = m.startInterfaceRelationController(topoStore)
	if err != nil {
		return err
	}

	err = m.startNorthboundServer()
	if err != nil {
		return err
	}
	return nil
}

// startSouthboundServer starts the northbound gRPC server
func (m *Manager) startNorthboundServer() error {
	log.Info("Starting NB server")
	s := northbound.NewServer(northbound.NewServerCfg(
		m.Config.CAPath,
		m.Config.KeyPath,
		m.Config.CertPath,
		int16(m.Config.GRPCPort),
		true,
		northbound.SecurityConfig{}))
	s.AddService(logging.Service{})

	doneCh := make(chan error)
	go func() {
		err := s.Serve(func(started string) {
			log.Info("Started NBI on ", started)
			close(doneCh)
		})
		if err != nil {
			doneCh <- err
		}
	}()
	return <-doneCh
}

func (m *Manager) startInterfaceController(topo topo.Store) error {
	portController := logicalinterface.NewController(topo)
	return portController.Start()
}

func (m *Manager) startInterfaceRelationController(topo topo.Store) error {
	portRelationController := logicalinterfacerelation.NewController(topo)
	return portRelationController.Start()
}

func (m *Manager) startLinkController(topo topo.Store) error {
	linkController := link.NewController(topo)
	return linkController.Start()
}

// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

// Package basic is a suite of basic functionality tests for the device provisioner
package basic

import (
	"context"
	fsimtopo "github.com/onosproject/fabric-sim/pkg/topo"
	"github.com/onosproject/helmit/pkg/helm"
	"github.com/onosproject/helmit/pkg/input"
	"github.com/onosproject/helmit/pkg/test"
	"github.com/onosproject/onos-api/go/onos/provisioner"
	libtest "github.com/onosproject/onos-lib-go/pkg/test"
	"github.com/onosproject/onos-test/pkg/onostest"
	"github.com/onosproject/topo-discovery/test/utils/charts"
	"google.golang.org/grpc"
	"os"
)

type testSuite struct {
	test.Suite
}

// TestSuite is the basic test suite
type TestSuite struct {
	testSuite
	fsimConn *grpc.ClientConn
}

// SetupTestSuite sets up the fabric simulator basic test suite
func (s *TestSuite) SetupTestSuite(c *input.Context) error {
	registry := c.GetArg("registry").String("")
	umbrella := charts.CreateUmbrellaRelease()
	err := umbrella.
		Set("global.image.registry", registry).
		Set("import.device-provisioner.enabled", true).
		Set("topo-discovery.image.tag", "latest").
		Set("import.topo-discovery.enabled", true).
		Set("import.onos-config.enabled", false).
		Install(true)
	if err != nil {
		return err
	}
	// Start fabric sim and load the test topology
	err = installChart("fabric-sim", registry, true)
	if err != nil {
		return err
	}
	s.fsimConn, err = libtest.CreateConnection("fabric-sim:5150", true)
	if err != nil {
		return err
	}
	err = fsimtopo.LoadTopology(s.fsimConn, "./test/basic/topo.yaml")
	if err != nil {
		return err
	}

	if err = createPipelineConfig(); err != nil {
		return err
	}

	err = installChart("link-local-agent", registry, false)
	if err != nil {
		return err
	}

	return nil
}

func installChart(name string, registry string, wait bool) error {
	return helm.Chart(name, onostest.OnosChartRepo).Release(name).
		Set("image.tag", "latest").
		Set("global.image.registry", registry).
		Set("agent.count", 4). // There are 4 devices in topo.yaml topology file
		Install(true)
}

func createPipelineConfig() error {
	conn, err := libtest.CreateConnection("device-provisioner:5150", false)
	if err != nil {
		return err
	}

	p4infoBytes, err := os.ReadFile("./test/basic/p4info.txt")
	if err != nil {
		return err
	}

	// Add pipeline config
	ctx := context.Background()
	provClient := provisioner.NewProvisionerServiceClient(conn)
	_, err = provClient.Add(ctx, &provisioner.AddConfigRequest{
		Config: &provisioner.Config{
			Record: &provisioner.ConfigRecord{
				ConfigID: pipelineConfigID,
				Kind:     provisioner.PipelineConfigKind,
			},
			Artifacts: map[string][]byte{
				provisioner.P4InfoType:   p4infoBytes,
				provisioner.P4BinaryType: p4infoBytes,
			},
		},
	})
	return err
}

// SPDX-FileCopyrightText: 2023-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

// Package basic is a suite of basic functionality tests for the device provisioner
package basic

import (
	fsimtopo "github.com/onosproject/fabric-sim/pkg/topo"
	"github.com/onosproject/helmit/pkg/helm"
	"github.com/onosproject/helmit/pkg/input"
	"github.com/onosproject/helmit/pkg/test"
	libtest "github.com/onosproject/onos-lib-go/pkg/test"
	"github.com/onosproject/onos-test/pkg/onostest"
)

type testSuite struct {
	test.Suite
}

// TestSuite is the basic test suite
type TestSuite struct {
	testSuite
}

const (
	topoName               = "onos-topo"
	fabricSimComponentName = "fabric-sim"
	topoDiscoveryName      = "topo-discovery"
)

// SetupTestSuite sets up the fabric simulator basic test suite
func (s *TestSuite) SetupTestSuite(c *input.Context) error {
	registry := c.GetArg("registry").String("")
	err := helm.Chart(fabricSimComponentName, onostest.OnosChartRepo).
		Release(fabricSimComponentName).
		Set("image.tag", "latest").
		Set("global.image.registry", registry).
		Install(false)
	if err != nil {
		return err
	}

	err = helm.Chart(topoName, onostest.OnosChartRepo).
		Release(topoName).
		Set("image.tag", "latest").
		Set("global.image.registry", registry).
		Install(true)
	if err != nil {
		return err
	}

	err = helm.Chart(topoDiscoveryName, onostest.OnosChartRepo).
		Release(topoDiscoveryName).
		Set("image.tag", "latest").
		Set("global.image.registry", registry).
		Install(true)
	if err != nil {
		return err
	}

	fsimConn, err := libtest.CreateConnection("fabric-sim:5150", true)
	if err != nil {
		return err
	}

	err = fsimtopo.LoadTopology(fsimConn, "./test/basic/topo.yaml")
	if err != nil {
		return err
	}

	return nil
}

// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

// Package main is an entry point for launching the topology discovery
package main

import (
	"github.com/onosproject/onos-lib-go/pkg/cli"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/topo-discovery/pkg/manager"
	"github.com/spf13/cobra"
)

var log = logging.GetLogger()

const (
	realmLabelFlag     = "realm-label"
	realmValueFlag     = "realm-value"
	topoAddressFlag    = "topo-address"
	defaultTopoAddress = "onos-topo:5150"
)

// The main entry point
func main() {
	cmd := &cobra.Command{
		Use:  "topo-discovery",
		RunE: runRootCommand,
	}
	cmd.Flags().String(realmLabelFlag, "pod", "label used to define the realm of devices over which the discovery should operate")
	cmd.Flags().String(realmValueFlag, "all", "value of the realm label of devices over which the discovery should operate")
	cmd.Flags().String(topoAddressFlag, defaultTopoAddress, "address:port or just :port of the onos-topo service")
	cli.AddServiceEndpointFlags(cmd, "discovery gRPC")
	cli.Run(cmd)
}

func runRootCommand(cmd *cobra.Command, args []string) error {
	realmLabel, _ := cmd.Flags().GetString(realmLabelFlag)
	realmValue, _ := cmd.Flags().GetString(realmValueFlag)
	topoAddress, _ := cmd.Flags().GetString(topoAddressFlag)

	flags, err := cli.ExtractServiceEndpointFlags(cmd)
	if err != nil {
		return err
	}

	log.Infof("Starting topo-discovery")
	cfg := manager.Config{
		RealmLabel:   realmLabel,
		RealmValue:   realmValue,
		TopoAddress:  topoAddress,
		ServiceFlags: flags,
	}

	return cli.RunDaemon(manager.NewManager(cfg))
}

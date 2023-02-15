// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

// Package main is an entry point for launching the topology discovery
package main

import (
	"github.com/onosproject/onos-lib-go/pkg/cli"
	"github.com/onosproject/onos-lib-go/pkg/logging"
	"github.com/onosproject/onos-net-lib/pkg/realm"
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
	realm.AddRealmFlags(cmd, "discovery")
	cmd.Flags().String(topoAddressFlag, defaultTopoAddress, "address:port or just :port of the onos-topo service")
	cli.AddServiceEndpointFlags(cmd, "discovery gRPC")
	cli.Run(cmd)
}

func runRootCommand(cmd *cobra.Command, args []string) error {
	topoAddress, _ := cmd.Flags().GetString(topoAddressFlag)
	realmOptions := realm.ExtractOptions(cmd)

	flags, err := cli.ExtractServiceEndpointFlags(cmd)
	if err != nil {
		return err
	}

	log.Infof("Starting topo-discovery")
	cfg := manager.Config{
		RealmOptions: realmOptions,
		TopoAddress:  topoAddress,
		ServiceFlags: flags,
	}

	return cli.RunDaemon(manager.NewManager(cfg))
}

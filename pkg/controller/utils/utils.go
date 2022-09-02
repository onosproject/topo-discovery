// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/uri"
)

// GetPortID gets port URI
func GetPortID(targetID string, portID string) topoapi.ID {
	opaque := targetID + "/" + portID
	return topoapi.ID(uri.NewURI(
		uri.WithOpaque(opaque)).String())
}

// GetContainPortRelationID  creates a CONTAIN port relation ID
func GetContainPortRelationID(targetEntityID, portEntityID topoapi.ID) topoapi.ID {
	opaque := targetEntityID + "/" + portEntityID
	return topoapi.ID(uri.NewURI(
		uri.WithOpaque(string(opaque))).String())
}

// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/uri"
)

// GetLogicalInterfaceID gets logical interface URI
func GetLogicalInterfaceID(targetID string, logicalInterfaceID string) topoapi.ID {
	opaque := targetID + "/" + logicalInterfaceID
	return topoapi.ID(uri.NewURI(
		uri.WithOpaque(opaque)).String())
}

// GetLinkID gets link ID
func GetLinkID(sourceInterfaceID, destInterfaceID string) topoapi.ID {
	opaque := sourceInterfaceID + "/" + destInterfaceID
	return topoapi.ID(uri.NewURI(
		uri.WithOpaque(opaque)).String())
}

// GetContainLogicalInterfaceRelationID  creates a CONTAIN port relation ID
func GetContainLogicalInterfaceRelationID(targetEntityID, logicalInterfaceEntityID topoapi.ID) topoapi.ID {
	opaque := targetEntityID + "/" + logicalInterfaceEntityID
	return topoapi.ID(uri.NewURI(
		uri.WithOpaque(string(opaque))).String())
}

// SPDX-FileCopyrightText: 2022-present Intel Corporation
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	topoapi "github.com/onosproject/onos-api/go/onos/topo"
	"github.com/onosproject/onos-lib-go/pkg/errors"
	"github.com/onosproject/onos-lib-go/pkg/uri"
	"strings"
)

// GetPhyInterfaceID gets logical interface URI
func GetPhyInterfaceID(targetID string, logicalInterfaceID string) topoapi.ID {
	opaque := targetID + "/" + logicalInterfaceID
	return topoapi.ID(uri.NewURI(
		uri.WithOpaque(opaque)).String())
}

// GetLinkID gets link ID
func GetLinkID(sourceInterfaceID, destInterfaceID topoapi.ID) topoapi.ID {
	opaque := sourceInterfaceID + ":" + destInterfaceID
	return topoapi.ID(uri.NewURI(
		uri.WithOpaque(string(opaque))).String())

}

// GetInterfaceIDsFromLinkID extracts source and destination interfaces given a link ID
func GetInterfaceIDsFromLinkID(linkID topoapi.ID) (topoapi.ID, topoapi.ID, error) {
	result := strings.Split(string(linkID), ":")
	if len(result) == 2 {
		return topoapi.ID(result[0]), topoapi.ID(result[1]), nil
	}
	return "", "", errors.NewInvalid("link ID is not valid")
}

// GetContainPhyInterfaceRelationID  creates a CONTAIN port relation ID
func GetContainPhyInterfaceRelationID(targetEntityID, logicalInterfaceEntityID topoapi.ID) topoapi.ID {
	opaque := targetEntityID + "/" + logicalInterfaceEntityID
	return topoapi.ID(uri.NewURI(
		uri.WithOpaque(string(opaque))).String())
}

// GetLinkOriginatesRelationID  creates a link originates relation ID
func GetLinkOriginatesRelationID(sourceInterfaceID, linkEntityID topoapi.ID) topoapi.ID {
	opaque := sourceInterfaceID + ":" + linkEntityID
	return topoapi.ID(uri.NewURI(
		uri.WithOpaque(string(opaque))).String())
}

// GetLinkTerminatesRelationID  creates a link terminates relation ID
func GetLinkTerminatesRelationID(linkEntityID topoapi.ID, destInterfaceID topoapi.ID) topoapi.ID {
	opaque := linkEntityID + ":" + destInterfaceID
	return topoapi.ID(uri.NewURI(
		uri.WithOpaque(string(opaque))).String())
}

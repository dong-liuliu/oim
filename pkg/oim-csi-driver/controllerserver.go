/*
Copyright 2017 The Kubernetes Authors.

SPDX-License-Identifier: Apache-2.0
*/

package oimcsidriver

import (
	"context"
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

func (od *oimDriver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	name := req.GetName()
	caps := req.GetVolumeCapabilities()

	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "Name missing in request")
	}
	if caps == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume Capabilities missing in request")
	}
	for _, cap := range caps {
		if cap.GetBlock() != nil {
			return nil, status.Error(codes.Unimplemented, "Block Volume not supported")
		}
		switch cap.GetAccessMode().GetMode() {
		case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER: // okay
		case csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY: // okay
		case csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY: // okay

		case csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER:
			// While in theory writing blocks on one node and reading them on others could work,
			// in practice caching effects might break that. Better don't allow it.
			return nil, status.Error(codes.Unimplemented, "multi-node reader, single writer not supported")
		case csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER:
			return nil, status.Error(codes.Unimplemented, "multi-node reader, multi-node writer not supported")
		default:
			return nil, status.Error(codes.Unimplemented, fmt.Sprintf("%s not supported", cap.GetAccessMode().GetMode()))
		}
	}
	if req.GetVolumeContentSource() != nil {
		return nil, status.Error(codes.Unimplemented, "snapshots not supported")
	}

	// Serialize operations per volume by name.
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "empty name")
	}
	volumeNameMutex.LockKey(name)
	defer volumeNameMutex.UnlockKey(name)

	actualBytes, err := od.backend.createVolume(ctx, name, req.GetCapacityRange().GetRequiredBytes())
	if err != nil {
		return nil, err
	}
	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			// We use the unique name also as ID.
			VolumeId:      name,
			CapacityBytes: actualBytes,
		},
	}, nil
}

func (od *oimDriver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	// Check arguments
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}

	// Volume ID is the same as the volume name in CreateVolume. Serialize by that.
	name := req.GetVolumeId()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID")
	}
	volumeNameMutex.LockKey(name)
	defer volumeNameMutex.UnlockKey(name)

	if err := od.backend.deleteVolume(ctx, name); err != nil {
		return nil, err
	}
	return &csi.DeleteVolumeResponse{}, nil
}

func (od *oimDriver) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (od *oimDriver) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (od *oimDriver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {

	// Check arguments
	if len(req.GetVolumeId()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID missing in request")
	}
	if req.GetVolumeCapabilities() == nil {
		return nil, status.Error(codes.InvalidArgument, "Volume capabilities missing in request")
	}

	// Volume ID is the same as the volume name in CreateVolume. Serialize by that.
	name := req.GetVolumeId()
	if name == "" {
		return nil, status.Error(codes.InvalidArgument, "empty volume ID")
	}
	volumeNameMutex.LockKey(name)
	defer volumeNameMutex.UnlockKey(name)

	// Check that volume exists.
	if err := od.backend.checkVolumeExists(ctx, req.GetVolumeId()); err != nil {
		return nil, err
	}

	confirmed := &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
		// We don't actually do any validation of these (don't even use them!).
		// It's also unclear from the spec what a CO would do with the validated
		// values, because both are opaque to the CO.
		VolumeContext: req.VolumeContext,
		Parameters:    req.Parameters,
	}
	for _, cap := range req.VolumeCapabilities {
		if cap.GetBlock() != nil {
			/* Known unsupported mode. Fail the validation. */
			return &csi.ValidateVolumeCapabilitiesResponse{Message: "Block Volume not supported"}, nil
		}
		if cap.GetMount() == nil {
			/* Must be something else, an unknown mode. Ignore it. */
			continue
		}
		// We could check fs type and mount flags for MountVolume, but let's assume that they are okay.
		// Now check the access mode.
		switch cap.GetAccessMode().GetMode() {
		case csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER: // okay
		case csi.VolumeCapability_AccessMode_SINGLE_NODE_READER_ONLY: // okay
		case csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY: // okay

		case csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER:
			// While in theory writing blocks on one node and reading them on others could work,
			// in practice caching effects might break that. Better don't allow it.
			return &csi.ValidateVolumeCapabilitiesResponse{Message: "multi-node reader, single writer not supported"}, nil
		case csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER:
			return &csi.ValidateVolumeCapabilitiesResponse{Message: "multi-node reader, multi-node writer not supported"}, nil
		default:
			/* unknown, not supported */
			continue
		}
		confirmed.VolumeCapabilities = append(confirmed.VolumeCapabilities, cap)
	}
	return &csi.ValidateVolumeCapabilitiesResponse{Confirmed: confirmed}, nil
}

func (od *oimDriver) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (od *oimDriver) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

// ControllerGetCapabilities implements the default GRPC callout.
// Default supports all capabilities
func (od *oimDriver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: od.cap,
	}, nil
}

func (od *oimDriver) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (od *oimDriver) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (od *oimDriver) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

func (od *oimDriver) ControllerExpandVolume(context.Context, *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, "")
}

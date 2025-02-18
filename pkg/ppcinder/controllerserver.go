package ppcinder

import (
	"fmt"
	"github.com/Frank-svg-dev/ppcluster-cinder-csi/pkg/ppcinder/openstackphy"
	"github.com/Frank-svg-dev/ppcluster-cinder-csi/pkg/pperror"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"github.com/kubernetes-csi/csi-lib-utils/protosanitizer"
	"golang.org/x/net/context"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/klog/v2"
)

const (
	cinderCSIClusterIDKey = "cinder.csi.ppcluster.org/cluster"
)

type controllerServer struct {
	Driver    *Driver
	OpPhycial openstackphy.OpenstackPhyIO
}

func (c *controllerServer) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, fmt.Sprintf("GetCapacity is not yet implemented"))
}

func (c *controllerServer) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	return nil, status.Error(codes.Unimplemented, fmt.Sprintf("GetCapacity is not yet implemented"))
}

func (c *controllerServer) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return nil, status.Error(codes.Unimplemented, fmt.Sprintf("GetCapacity is not yet implemented"))
}

func (c *controllerServer) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	klog.V(4).Infof("CreateVolume: called with args %+v", protosanitizer.StripSecrets(*req))

	// Volume Name
	volName := req.GetName()
	volCapabilities := req.GetVolumeCapabilities()

	if len(volName) == 0 {
		return nil, status.Error(codes.InvalidArgument, "[CreateVolume] missing Volume Name")
	}

	if volCapabilities == nil {
		return nil, status.Error(codes.InvalidArgument, "[CreateVolume] missing Volume capability")
	}

	// Volume Size - Default is 1 GiB
	volSizeBytes := int64(1 * 1024 * 1024 * 1024)
	if req.GetCapacityRange() != nil {
		volSizeBytes = int64(req.GetCapacityRange().GetRequiredBytes())
	}
	volSizeGB := int(RoundUpSize(volSizeBytes, 1024*1024*1024))

	// Volume Type
	volType := req.GetParameters()["type"]

	var volAvailability string

	// First check if volAvailability is already specified, if not get preferred from Topology
	// Required, incase vol AZ is different from node AZ
	volAvailability = req.GetParameters()["availability"]

	if len(volAvailability) == 0 {
		// Check from Topology
		if req.GetAccessibilityRequirements() != nil {
			volAvailability = "default-az"
		}
	}

	cloud := c.OpPhycial
	//ignoreVolumeAZ := true

	// Verify a volume with the provided name doesn't already exist for this tenant
	volumes, err := cloud.GetVolumesByName(volName)
	if err != nil {
		klog.Errorf("Failed to query for existing Volume during CreateVolume: %v", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("Failed to get volumes: %s", err))
	}

	if len(volumes) == 1 {
		if volSizeGB != volumes[0].Size {
			return nil, status.Error(codes.AlreadyExists, "Volume Already exists with same name and different capacity")
		}
		klog.V(4).Infof("Volume %s already exists in Availability Zone: %s of size %d GiB", volumes[0].ID, volumes[0].AvailabilityZone, volumes[0].Size)
		return getCreateVolumeResponse(&volumes[0], true, req.GetAccessibilityRequirements()), nil
	} else if len(volumes) > 1 {
		klog.V(3).Infof("found multiple existing volumes with selected name (%s) during create", volName)
		return nil, status.Error(codes.Internal, "Multiple volumes reported by Cinder with same name")

	}

	// Volume Create
	properties := map[string]string{cinderCSIClusterIDKey: c.Driver.cluster}
	//Tag volume with metadata if present: https://github.com/kubernetes-csi/external-provisioner/pull/399
	for _, mKey := range []string{"csi.storage.k8s.io/pvc/name", "csi.storage.k8s.io/pvc/namespace", "csi.storage.k8s.io/pv/name"} {
		if v, ok := req.Parameters[mKey]; ok {
			properties[mKey] = v
		}
	}
	//content := req.GetVolumeContentSource()
	//var snapshotID string
	//var sourcevolID string
	//
	//if content != nil && content.GetSnapshot() != nil {
	//	snapshotID = content.GetSnapshot().GetSnapshotId()
	//	_, err := cloud.GetSnapshotByID(snapshotID)
	//	if err != nil {
	//		if cpoerrors.IsNotFound(err) {
	//			return nil, status.Errorf(codes.NotFound, "VolumeContentSource Snapshot %s not found", snapshotID)
	//		}
	//		return nil, status.Errorf(codes.Internal, "Failed to retrieve the snapshot %s: %v", snapshotID, err)
	//	}
	//}
	//
	//if content != nil && content.GetVolume() != nil {
	//	sourcevolID = content.GetVolume().GetVolumeId()
	//	_, err := cloud.GetVolume(sourcevolID)
	//	if err != nil {
	//		if cpoerrors.IsNotFound(err) {
	//			return nil, status.Errorf(codes.NotFound, "Source Volume %s not found", sourcevolID)
	//		}
	//		return nil, status.Errorf(codes.Internal, "Failed to retrieve the source volume %s: %v", sourcevolID, err)
	//	}
	//}

	vol, err := cloud.CreateVolume(volName, volSizeGB, volType, volAvailability, "", "", &properties)

	if err != nil {
		klog.Errorf("Failed to CreateVolume: %v", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("CreateVolume failed with error %v", err))

	}

	klog.V(4).Infof("CreateVolume: Successfully created volume %s in Availability Zone: %s of size %d GiB", vol.ID, vol.AvailabilityZone, vol.Size)

	return getCreateVolumeResponse(vol, true, req.GetAccessibilityRequirements()), nil
}

func (c *controllerServer) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	klog.V(4).Infof("DeleteVolume: called with args %+v", protosanitizer.StripSecrets(*req))

	// Volume Delete
	volID := req.GetVolumeId()
	if len(volID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "DeleteVolume Volume ID must be provided")
	}
	err := c.OpPhycial.DeleteVolume(volID)
	if err != nil {
		if pperror.IsNotFound(err) {
			klog.V(3).Infof("Volume %s is already deleted.", volID)
			return &csi.DeleteVolumeResponse{}, nil
		}
		klog.Errorf("Failed to DeleteVolume: %v", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("DeleteVolume failed with error %v", err))
	}

	klog.V(4).Infof("DeleteVolume: Successfully deleted volume %s", volID)

	return &csi.DeleteVolumeResponse{}, nil
}

func (c *controllerServer) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	klog.V(4).Infof("ControllerPublishVolume: called with args %+v", protosanitizer.StripSecrets(*req))

	// Volume Attach
	instanceID := req.GetNodeId()
	volumeID := req.GetVolumeId()
	volumeCapability := req.GetVolumeCapability()

	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "[ControllerPublishVolume] Volume ID must be provided")
	}
	if len(instanceID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "[ControllerPublishVolume] Instance ID must be provided")
	}
	if volumeCapability == nil {
		return nil, status.Error(codes.InvalidArgument, "[ControllerPublishVolume] Volume capability must be provided")
	}

	_, err := c.OpPhycial.GetVolume(volumeID)
	if err != nil {
		if pperror.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "[ControllerPublishVolume] Volume %s not found", volumeID)
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("[ControllerPublishVolume] get volume failed with error %v", err))
	}

	err = c.OpPhycial.AttachVolume(instanceID, volumeID)
	if err != nil {
		klog.Errorf("Failed to AttachVolume: %v", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("[ControllerPublishVolume] Attach Volume failed with error %v", err))

	}

	err = c.OpPhycial.WaitDiskAttached(instanceID, volumeID)
	if err != nil {
		klog.Errorf("Failed to WaitDiskAttached: %v", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("[ControllerPublishVolume] failed to attach volume: %v", err))
	}

	devicePath, err := c.OpPhycial.GetAttachmentDiskPath(instanceID, volumeID)
	if err != nil {
		klog.Errorf("Failed to GetAttachmentDiskPath: %v", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("[ControllerPublishVolume] failed to get device path of attached volume : %v", err))
	}

	klog.V(4).Infof("ControllerPublishVolume %s on %s is successful", volumeID, instanceID)

	// Publish Volume Info
	pvInfo := map[string]string{}
	pvInfo["DevicePath"] = devicePath

	return &csi.ControllerPublishVolumeResponse{
		PublishContext: pvInfo,
	}, nil

}

func (c *controllerServer) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	klog.V(4).Infof("ControllerUnpublishVolume: called with args %+v", protosanitizer.StripSecrets(*req))

	// Volume Detach
	instanceID := req.GetNodeId()
	volumeID := req.GetVolumeId()

	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "[ControllerUnpublishVolume] Volume ID must be provided")
	}

	//err = cs.Cloud.DetachVolume(instanceID, volumeID)
	err := c.OpPhycial.DetachVolume(instanceID, volumeID)
	if err != nil {
		if pperror.IsNotFound(err) {
			klog.V(3).Infof("ControllerUnpublishVolume assuming volume %s is detached, because it does not exist", volumeID)
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		klog.Errorf("Failed to DetachVolume: %v", err)
		return nil, status.Error(codes.Internal, fmt.Sprintf("ControllerUnpublishVolume Detach Volume failed with error %v", err))
	}

	err = c.OpPhycial.WaitDiskDetached(instanceID, volumeID)
	if err != nil {
		klog.Errorf("Failed to WaitDiskDetached: %v", err)
		if pperror.IsNotFound(err) {
			klog.V(3).Infof("ControllerUnpublishVolume assuming volume %s is detached, because it was deleted in the meanwhile", volumeID)
			return &csi.ControllerUnpublishVolumeResponse{}, nil
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("ControllerUnpublishVolume failed with error %v", err))
	}

	klog.V(4).Infof("ControllerUnpublishVolume %s on %s", volumeID, instanceID)

	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (c *controllerServer) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	reqVolCap := req.GetVolumeCapabilities()

	if reqVolCap == nil || len(reqVolCap) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume Capabilities must be provided")
	}
	volumeID := req.GetVolumeId()

	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "ValidateVolumeCapabilities Volume ID must be provided")
	}

	_, err := c.OpPhycial.GetVolume(volumeID)
	if err != nil {
		if pperror.IsNotFound(err) {
			return nil, status.Error(codes.NotFound, fmt.Sprintf("ValidateVolumeCapabiltites Volume %s not found", volumeID))
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("ValidateVolumeCapabiltites %v", err))
	}

	for _, cap := range reqVolCap {
		if cap.GetAccessMode().GetMode() != c.Driver.vcap[0].Mode {
			return &csi.ValidateVolumeCapabilitiesResponse{Message: "Requested Volume Capabilty not supported"}, nil
		}
	}

	// Cinder CSI driver currently supports one mode only
	resp := &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: []*csi.VolumeCapability{
				{
					AccessMode: c.Driver.vcap[0],
				},
			},
		},
	}

	return resp, nil
}

func (c *controllerServer) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	if req.MaxEntries < 0 {
		return nil, status.Error(codes.InvalidArgument, fmt.Sprintf(
			"[ListVolumes] Invalid max entries request %v, must not be negative ", req.MaxEntries))
	}
	maxEntries := int(req.MaxEntries)

	vlist, nextPageToken, err := c.OpPhycial.ListVolumes(maxEntries, req.StartingToken)
	if err != nil {
		klog.Errorf("Failed to ListVolumes: %v", err)
		if pperror.IsInvalidError(err) {
			return nil, status.Errorf(codes.Aborted, "[ListVolumes] Invalid request: %v", err)
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("ListVolumes failed with error %v", err))
	}

	var ventries []*csi.ListVolumesResponse_Entry
	for _, v := range vlist {
		ventry := csi.ListVolumesResponse_Entry{
			Volume: &csi.Volume{
				VolumeId:      v.ID,
				CapacityBytes: int64(v.Size * 1024 * 1024 * 1024),
			},
		}

		status := &csi.ListVolumesResponse_VolumeStatus{}
		for _, attachment := range v.Attachments {
			status.PublishedNodeIds = append(status.PublishedNodeIds, attachment.ServerID)
		}
		ventry.Status = status

		ventries = append(ventries, &ventry)
	}
	return &csi.ListVolumesResponse{
		Entries:   ventries,
		NextToken: nextPageToken,
	}, nil
}

func (c *controllerServer) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return nil, status.Error(codes.Unimplemented, fmt.Sprintf("GetCapacity is not yet implemented"))
}

func (c *controllerServer) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	klog.V(5).Infof("Using default ControllerGetCapabilities")

	return &csi.ControllerGetCapabilitiesResponse{
		Capabilities: c.Driver.cscap,
	}, nil
}

func (c *controllerServer) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	return nil, status.Error(codes.Unimplemented, fmt.Sprintf("GetCapacity is not yet implemented"))
}

func (c *controllerServer) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	klog.V(4).Infof("ControllerGetVolume: called with args %+v", protosanitizer.StripSecrets(*req))

	volumeID := req.GetVolumeId()

	if len(volumeID) == 0 {
		return nil, status.Error(codes.InvalidArgument, "Volume ID not provided")
	}

	volume, err := c.OpPhycial.GetVolume(volumeID)
	if err != nil {
		if pperror.IsNotFound(err) {
			return nil, status.Errorf(codes.NotFound, "Volume %s not found", volumeID)
		}
		return nil, status.Error(codes.Internal, fmt.Sprintf("ControllerGetVolume failed with error %v", err))
	}

	ventry := csi.ControllerGetVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      volumeID,
			CapacityBytes: int64(volume.Size * 1024 * 1024 * 1024),
		},
	}

	status := &csi.ControllerGetVolumeResponse_VolumeStatus{}
	for _, attachment := range volume.Attachments {
		status.PublishedNodeIds = append(status.PublishedNodeIds, attachment.ServerID)
	}
	ventry.Status = status

	return &ventry, nil
}
func getCreateVolumeResponse(vol *volumes.Volume, ignoreVolumeAZ bool, accessibleTopologyReq *csi.TopologyRequirement) *csi.CreateVolumeResponse {

	var volsrc *csi.VolumeContentSource

	if vol.SnapshotID != "" {
		volsrc = &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Snapshot{
				Snapshot: &csi.VolumeContentSource_SnapshotSource{
					SnapshotId: vol.SnapshotID,
				},
			},
		}
	}

	if vol.SourceVolID != "" {
		volsrc = &csi.VolumeContentSource{
			Type: &csi.VolumeContentSource_Volume{
				Volume: &csi.VolumeContentSource_VolumeSource{
					VolumeId: vol.SourceVolID,
				},
			},
		}
	}

	var accessibleTopology []*csi.Topology
	// If ignore-volume-az is true , dont set the accessible topology to volume az,
	// use from preferred topologies instead.
	if ignoreVolumeAZ {
		if accessibleTopologyReq != nil {
			accessibleTopology = accessibleTopologyReq.GetPreferred()
		}
	}

	resp := &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:           vol.ID,
			CapacityBytes:      int64(vol.Size * 1024 * 1024 * 1024),
			AccessibleTopology: accessibleTopology,
			ContentSource:      volsrc,
		},
	}

	return resp

}

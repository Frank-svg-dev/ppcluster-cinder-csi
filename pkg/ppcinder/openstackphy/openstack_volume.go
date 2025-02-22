package openstackphy

import (
	"fmt"
	"github.com/Frank-svg-dev/ppcluster-cinder-csi/pkg/pperror"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/attachments"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"github.com/gophercloud/gophercloud/v2/pagination"
	"golang.org/x/net/context"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/klog/v2"
	"net/url"
	"time"
)

const (
	VolumeAvailableStatus    = "available"
	VolumeInUseStatus        = "in-use"
	operationFinishInitDelay = 1 * time.Second
	operationFinishFactor    = 1.1
	operationFinishSteps     = 10
	diskAttachInitDelay      = 1 * time.Second
	diskAttachFactor         = 1.2
	diskAttachSteps          = 15
	diskDetachInitDelay      = 1 * time.Second
	diskDetachFactor         = 1.2
	diskDetachSteps          = 13
	volumeDescription        = "Created by ppcluster Cinder CSI driver For Physical"
)

var volumeErrorStates = [...]string{"error", "error_extending", "error_deleting"}

func (op *OpenStackPhy) CreateVolume(name string, size int, vtype, availability string, snapshotID string, sourcevolID string, tags *map[string]string) (*volumes.Volume, error) {

	ctx := context.Background()

	opts := &volumes.CreateOpts{
		Name:             name,
		Size:             size,
		VolumeType:       vtype,
		AvailabilityZone: availability,
		Description:      volumeDescription,
		SnapshotID:       snapshotID,
		SourceVolID:      sourcevolID,
	}
	if tags != nil {
		opts.Metadata = *tags
	}

	vol, err := volumes.Create(ctx, op.blockstorage, opts, nil).Extract()
	if err != nil {
		return nil, err
	}

	return vol, nil
}

func (op *OpenStackPhy) DeleteVolume(volumeID string) error {
	ctx := context.Background()
	used, err := op.diskIsUsed(volumeID)
	if err != nil {
		return err
	}
	if used {
		return fmt.Errorf("Cannot delete the volume %q, it's still attached to a node", volumeID)
	}

	err = volumes.Delete(ctx, op.blockstorage, volumeID, nil).ExtractErr()
	return err
}

func (op *OpenStackPhy) AttachVolume(instanceID, volumeID string) error {
	ctx := context.Background()

	connector, err := op.GetConnector(instanceID)
	if err != nil {
		klog.ErrorS(err, "Failed to get connector", "instance_id", instanceID)
		return err
	}

	attachment_instance_id, err := op.GetNodeIdForECS(instanceID)
	if err != nil {
		klog.ErrorS(err, "Failed to create attachment instance_uuid", "instance_id", instanceID)
	}

	op.blockstorage.Microversion = "3.54"

	attachment, err := attachments.Create(ctx, op.blockstorage, attachments.CreateOpts{
		VolumeUUID:   volumeID,
		InstanceUUID: attachment_instance_id,
		Connector:    connector,
	}).Extract()

	if err != nil {
		return err
	}

	scanVolume, err := op.connectVolume(attachment.ConnectionInfo, instanceID)
	if err != nil {
		klog.Error("Failed to connect volume ", volumeID)
		klog.Info("Starting unreserve volume ", volumeID)
		if err := attachments.Delete(ctx, op.blockstorage, attachment.ID).ExtractErr(); err != nil {
			klog.Error("Failed to unreserve volume ", volumeID)
			return err
		}
		return err
	}

	connector["mountpoint"] = scanVolume["path"]
	connector["mode"] = "rw"

	if err := attachments.Update(ctx, op.blockstorage, attachment.ID, attachments.UpdateOpts{
		connector,
	}).Err; err != nil {
		klog.Error("Failed to update attachment volume ", volumeID)
		klog.Info("Starting unreserve volume ", volumeID)
		if err := op.disconnectVolume(attachment.ConnectionInfo, instanceID); err != nil {
			klog.Error("Failed to disconnect volume ", volumeID)
			return err
		}
		if err := attachments.Delete(ctx, op.blockstorage, attachment.ID).ExtractErr(); err != nil {
			klog.Error("Failed to unreserve volume ", volumeID)
			return err
		}
		return err
	}

	if err := attachments.Complete(ctx, op.blockstorage, attachment.ID).Err; err != nil {
		klog.Error("attachment complete failed ", volumeID)
		return err
	}

	return nil
}

func (op *OpenStackPhy) DetachVolume(instanceID, volumeID string) error {

	ctx := context.Background()

	op.blockstorage.Microversion = "3.54"

	if err := volumes.BeginDetaching(ctx, op.blockstorage, volumeID).Err; err != nil {
		return err
	}

	klog.Info("volume %s begin detach ok", volumeID)

	volume, err := volumes.Get(ctx, op.blockstorage, volumeID).Extract()

	if err != nil {
		klog.Info("get volume %s failed", volumeID)
		return err
	}

	attachment, err := attachments.Get(ctx, op.blockstorage, volume.Attachments[0].AttachmentID).Extract()

	if err != nil {
		klog.Error("get volume attachment failed ", volumeID)
		return err
	}

	if err := op.disconnectVolume(attachment.ConnectionInfo, instanceID); err != nil {
		klog.Error("Failed to disconnect volume ", volumeID)
		return err
	}

	if err := attachments.Delete(ctx, op.blockstorage, attachment.ID).Err; err != nil {
		klog.Info("volume %s detach failed", volumeID)
		return err
	}

	klog.Info("volume %s detach ok", volumeID)

	return nil
}

func (op *OpenStackPhy) GetVolume(volumeID string) (*volumes.Volume, error) {
	ctx := context.Background()
	vol, err := volumes.Get(ctx, op.blockstorage, volumeID).Extract()
	if err != nil {
		return nil, err
	}

	return vol, nil
}

func (op *OpenStackPhy) ListVolumes(limit int, startingToken string) ([]volumes.Volume, string, error) {
	ctx := context.Background()

	var nextPageToken string
	var vols []volumes.Volume

	opts := volumes.ListOpts{Limit: limit, Marker: startingToken}
	err := volumes.List(op.blockstorage, opts).EachPage(ctx, func(ctx2 context.Context, page pagination.Page) (bool, error) {
		var err error

		vols, err = volumes.ExtractVolumes(page)
		if err != nil {
			return false, err
		}

		nextPageURL, err := page.NextPageURL()
		if err != nil {
			return false, err
		}

		if nextPageURL != "" {
			queryParams, err := url.ParseQuery(nextPageURL)
			if err != nil {
				return false, err
			}
			nextPageToken = queryParams.Get("marker")
		}

		return false, nil
	})
	if err != nil {
		return nil, nextPageToken, err
	}

	return vols, nextPageToken, nil
}

func (op *OpenStackPhy) GetVolumesByName(name string) ([]volumes.Volume, error) {
	ctx := context.Background()
	op.blockstorage.Microversion = "3.34"

	opts := volumes.ListOpts{Name: name}
	pages, err := volumes.List(op.blockstorage, opts).AllPages(ctx)
	if err != nil {
		return nil, err
	}

	vols, err := volumes.ExtractVolumes(pages)
	if err != nil {
		return nil, err
	}

	return vols, nil
}

func (op *OpenStackPhy) GetMaxVolLimit() int64 {
	return defaultMaxVolAttachLimit
}

func (op *OpenStackPhy) GetAttachmentDiskPath(instanceID, volumeID string) (string, error) {
	volume, err := op.GetVolume(volumeID)
	if err != nil {
		return "", err
	}
	instanceID, err = op.GetNodeIdForECS(instanceID)

	if err != nil {
		return "", err
	}

	if volume.Status != VolumeInUseStatus {
		return "", fmt.Errorf("can not get device path of volume %s, its status is %s ", volume.Name, volume.Status)
	}

	if len(volume.Attachments) > 0 {
		for _, att := range volume.Attachments {
			if att.ServerID == instanceID {
				return att.Device, nil
			}
		}
		return "", fmt.Errorf("disk %q is not attached to compute: %q", volumeID, instanceID)
	}
	return "", fmt.Errorf("volume %s has no Attachments", volumeID)
}

func (op *OpenStackPhy) diskIsUsed(volumeID string) (bool, error) {
	volume, err := op.GetVolume(volumeID)
	if err != nil {
		return false, err
	}

	if len(volume.Attachments) > 0 {
		return volume.Attachments[0].ServerID != "", nil
	}

	return false, nil
}

func (op *OpenStackPhy) diskIsAttached(instanceID, volumeID string) (bool, error) {
	volume, err := op.GetVolume(volumeID)
	if err != nil {
		return false, err
	}
	instanceID, err = op.GetNodeIdForECS(instanceID)
	if err != nil {
		return false, err
	}
	for _, att := range volume.Attachments {
		if att.ServerID == instanceID {
			return true, nil
		}
	}
	return false, nil
}

func (op *OpenStackPhy) WaitDiskDetached(instanceID, volumeID string) error {
	backoff := wait.Backoff{
		Duration: diskDetachInitDelay,
		Factor:   diskDetachFactor,
		Steps:    diskDetachSteps,
	}

	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		attached, err := op.diskIsAttached(instanceID, volumeID)
		if err != nil {
			return false, err
		}
		return !attached, nil
	})

	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("Volume %q failed to detach within the alloted time", volumeID)
	}

	return err
}

// WaitDiskAttached waits for attched
func (op *OpenStackPhy) WaitDiskAttached(instanceID string, volumeID string) error {
	backoff := wait.Backoff{
		Duration: diskAttachInitDelay,
		Factor:   diskAttachFactor,
		Steps:    diskAttachSteps,
	}

	err := wait.ExponentialBackoff(backoff, func() (bool, error) {
		attached, err := op.diskIsAttached(instanceID, volumeID)
		if err != nil && !pperror.IsNotFound(err) {
			// if this is a race condition indicate the volume is deleted
			// during sleep phase, ignore the error and return attach=false
			return false, err
		}
		return attached, nil
	})

	if err == wait.ErrWaitTimeout {
		err = fmt.Errorf("Volume %q failed to be attached within the alloted time", volumeID)
	}

	return err
}

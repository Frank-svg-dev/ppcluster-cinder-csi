package openstackphy

import (
	"github.com/gophercloud/gophercloud/v2"
	"github.com/gophercloud/gophercloud/v2/openstack"
	"github.com/gophercloud/gophercloud/v2/openstack/blockstorage/v3/volumes"
	"github.com/spf13/pflag"
	"golang.org/x/net/context"
)

const defaultMaxVolAttachLimit int64 = 256

var userAgentData []string

// AddExtraFlags is called by the main package to add component specific command line flags
func AddExtraFlags(fs *pflag.FlagSet) {
	fs.StringArrayVar(&userAgentData, "user-agent", nil, "Extra data to add to gophercloud user-agent. Use multiple times to add more than one component.")
}

type OpenstackPhyIO interface {
	//CheckBlockStorageAPI() error
	CreateVolume(name string, size int, vtype, availability string, snapshotID string, sourcevolID string, tags *map[string]string) (*volumes.Volume, error)
	DeleteVolume(volumeID string) error
	AttachVolume(instanceID, volumeID string) error
	ListVolumes(limit int, startingToken string) ([]volumes.Volume, string, error)
	WaitDiskAttached(instanceID string, volumeID string) error
	DetachVolume(instanceID, volumeID string) error
	WaitDiskDetached(instanceID string, volumeID string) error
	//WaitVolumeTargetStatus(volumeID string, tStatus []string) error
	GetAttachmentDiskPath(instanceID, volumeID string) (string, error)
	GetVolume(volumeID string) (*volumes.Volume, error)
	GetVolumesByName(name string) ([]volumes.Volume, error)
	//ExpandVolume(volumeID string, status string, size int) error
	GetMaxVolLimit() int64
}

type OpenStackPhy struct {
	blockstorage *gophercloud.ServiceClient
}

func GetOpenStackCinderClient() (OpenstackPhyIO, error) {
	ctx := context.Background()
	providerClient, err := openstack.AuthenticatedClient(ctx, gophercloud.AuthOptions{
		IdentityEndpoint: "http://keystone.openstack.svc.cluster.local/v3",
		Username:         "admin",
		Password:         "Admin@OPS20!8",
		DomainName:       "Default",
		TenantName:       "admin",
	})
	if err != nil {
		return nil, err
	}

	epOpts := gophercloud.EndpointOpts{
		Region: "RegionOne",
	}

	cinderClient, err := openstack.NewBlockStorageV3(providerClient, epOpts)

	if err != nil {
		return nil, err
	}

	opPhy := OpenStackPhy{
		blockstorage: cinderClient,
	}

	return &opPhy, nil
}

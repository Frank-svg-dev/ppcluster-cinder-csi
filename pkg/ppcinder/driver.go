package ppcinder

import (
	"github.com/Frank-svg-dev/ppcluster-cinder-csi/pkg/mount"
	"github.com/Frank-svg-dev/ppcluster-cinder-csi/pkg/ppcinder/openstackphy"
	"github.com/container-storage-interface/spec/lib/go/csi"
	"k8s.io/klog/v2"
)

const (
	// DefaultDriverName defines the name that is used in Kubernetes and the CSI
	// system for the canonical, official name of this plugin
	DefaultDriverName        = "cinder.csi.ppcluster.com"
	defaultMaxVolumesPerNode = 7
	specVersion              = "1.11.0"
	Version                  = "v0.0.1"
	topologyKey              = "topology." + DefaultDriverName + "/zone"
)

// BaseDriver implements the following CSI interfaces:
//
//	csi.IdentityServer
//	csi.ControllerServer
//	csi.NodeServer

type Driver struct {
	name        string
	fqVersion   string
	endpoint    string
	cloudconfig string
	cluster     string

	ids *identityServer
	cs  *controllerServer
	ns  *nodeServer

	vcap  []*csi.VolumeCapability_AccessMode
	cscap []*csi.ControllerServiceCapability
	nscap []*csi.NodeServiceCapability
}

func NewDriver(endpoint, cluster string) *Driver {
	d := &Driver{}
	d.name = DefaultDriverName
	d.fqVersion = Version
	d.endpoint = endpoint
	d.cluster = cluster

	klog.Info("BaseDriver: ", d.name)
	klog.Info("BaseDriver version: ", d.fqVersion)
	klog.Info("CSI Spec version: ", specVersion)

	d.AddControllerServiceCapabilities([]csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_LIST_VOLUMES,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_PUBLISH_UNPUBLISH_VOLUME,
		//csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		//csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
		//csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		//csi.ControllerServiceCapability_RPC_CLONE_VOLUME,
		//csi.ControllerServiceCapability_RPC_LIST_VOLUMES_PUBLISHED_NODES,
		csi.ControllerServiceCapability_RPC_GET_VOLUME,
	})

	d.AddVolumeCapabilityAccessModes([]csi.VolumeCapability_AccessMode_Mode{
		csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
	})

	d.AddNodeServiceCapabilities([]csi.NodeServiceCapability_RPC_Type{
		//csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		//csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
		csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
	})

	return d
}

func (d *Driver) AddControllerServiceCapabilities(cl []csi.ControllerServiceCapability_RPC_Type) {
	var csc []*csi.ControllerServiceCapability

	for _, c := range cl {
		klog.Infof("Enabling controller service capability: %v", c.String())
		csc = append(csc, NewControllerServiceCapability(c))
	}

	d.cscap = csc

	return
}

func (d *Driver) AddVolumeCapabilityAccessModes(vc []csi.VolumeCapability_AccessMode_Mode) []*csi.VolumeCapability_AccessMode {
	var vca []*csi.VolumeCapability_AccessMode
	for _, c := range vc {
		klog.Infof("Enabling volume access mode: %v", c.String())
		vca = append(vca, NewVolumeCapabilityAccessMode(c))
	}
	d.vcap = vca
	return vca
}

func (d *Driver) AddNodeServiceCapabilities(nl []csi.NodeServiceCapability_RPC_Type) error {
	var nsc []*csi.NodeServiceCapability
	for _, n := range nl {
		klog.Infof("Enabling node service capability: %v", n.String())
		nsc = append(nsc, NewNodeServiceCapability(n))
	}
	d.nscap = nsc
	return nil
}

func (d *Driver) GetVolumeCapabilityAccessModes() []*csi.VolumeCapability_AccessMode {
	return d.vcap
}

func (d *Driver) SetupDriver(ophy openstackphy.OpenstackPhyIO, nm mount.IMount) {
	d.ids = NewIdentityServer(d)
	d.cs = NewControllerServer(d, ophy)
	d.ns = NewNodeServer(d, ophy, nm)
}

func (d *Driver) Run() {
	klog.Infof("ids %s", d.ids)
	klog.Infof("cs %s", d.cs)
	klog.Infof("ns %s", d.ns)
	RunControllerandNodePublishServer(d.endpoint, d.ids, d.cs, d.ns)
}

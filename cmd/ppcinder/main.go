package main

import (
	"flag"
	"github.com/Frank-svg-dev/ppcluster-cinder-csi/pkg/mount"
	"github.com/Frank-svg-dev/ppcluster-cinder-csi/pkg/ppcinder"
	"github.com/Frank-svg-dev/ppcluster-cinder-csi/pkg/ppcinder/openstackphy"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"k8s.io/component-base/cli"
	"k8s.io/klog/v2"
	"os"
)

var (
	endpoint    string
	nodeID      string
	cloudconfig []string
	cluster     string
)

func init() {
	flag.Set("logtostderr", "true")
}

func main() {

	flag.CommandLine.Parse([]string{})

	cmd := &cobra.Command{
		Use:   "Cinder",
		Short: "CSI based Cinder driver For Physical env",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Glog requires this otherwise it complains.
			flag.CommandLine.Parse(nil)

			// This is a temporary hack to enable proper logging until upstream dependencies
			// are migrated to fully utilize klog instead of glog.
			klogFlags := flag.NewFlagSet("klog", flag.ExitOnError)
			klog.InitFlags(klogFlags)

			// Sync the glog and klog flags.
			cmd.Flags().VisitAll(func(f1 *pflag.Flag) {
				f2 := klogFlags.Lookup(f1.Name)
				if f2 != nil {
					value := f1.Value.String()
					f2.Value.Set(value)
				}
			})
		},
		Run: func(cmd *cobra.Command, args []string) {
			handle()
		},
	}

	cmd.Flags().AddGoFlagSet(flag.CommandLine)

	cmd.PersistentFlags().StringVar(&nodeID, "nodeid", "", "node id")
	cmd.PersistentFlags().MarkDeprecated("nodeid", "This flag would be removed in future. Currently, the value is ignored by the driver")

	cmd.PersistentFlags().StringVar(&endpoint, "endpoint", "", "CSI endpoint")
	cmd.MarkPersistentFlagRequired("endpoint")

	cmd.PersistentFlags().StringSliceVar(&cloudconfig, "cloud-config", nil, "CSI driver cloud config. This option can be given multiple times")
	cmd.MarkPersistentFlagRequired("cloud-config")

	cmd.PersistentFlags().StringVar(&cluster, "cluster", "", "The identifier of the cluster that the plugin is running in.")

	openstackphy.AddExtraFlags(pflag.CommandLine)

	code := cli.Run(cmd)
	os.Exit(code)
}

func handle() {

	// Initialize cloud
	d := ppcinder.NewDriver(endpoint, cluster)
	ophy, err := openstackphy.GetOpenStackCinderClient()
	if err != nil {
		klog.Warningf("Failed to GetOpenStackProvider: %v", err)
		return
	}
	//Initialize mount
	mount := mount.GetMountProvider()

	d.SetupDriver(ophy, mount)
	d.Run()
}

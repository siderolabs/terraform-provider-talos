package main

import (
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	"github.com/talos-systems/talos/cmd/talosctl/cmd/mgmt"
	"github.com/talos-systems/talos/pkg/machinery/config/encoder"
	"github.com/talos-systems/talos/pkg/machinery/config/types/v1alpha1/generate"
	"github.com/talos-systems/talos/pkg/machinery/constants"
	"gopkg.in/yaml.v3"
)

func resourceServer() *schema.Resource {
	return &schema.Resource{
		Create: resourceServerCreate,
		Read:   resourceServerRead,
		Delete: resourceServerDelete,

		Schema: map[string]*schema.Schema{
			"cluster_name": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"cluster_endpoint": {
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"common_config_patches": {
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Required: false,
				Optional: true,
				ForceNew: true,
			},

			"control_plane_config_patches": {
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Required: false,
				Optional: true,
				ForceNew: true,
			},

			"worker_config_patches": {
				Type: schema.TypeList,
				Elem: &schema.Schema{
					Type: schema.TypeString,
				},
				Required: false,
				Optional: true,
				ForceNew: true,
			},

			"control_plane_machine_configuration": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"worker_machine_configuration": {
				Type:     schema.TypeString,
				Computed: true,
			},

			"talosconfig": {
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceServerCreate(d *schema.ResourceData, m interface{}) error {
	clusterName := d.Get("cluster_name").(string)

	clusterEndpoint := d.Get("cluster_endpoint").(string)

	configPatchesRaw := d.Get("common_config_patches").([]interface{})

	configPatches := make([]string, len(configPatchesRaw))
	for i, raw := range configPatchesRaw {
		configPatches[i] = raw.(string)
	}

	controlPlaneConfigPatchesRaw := d.Get("control_plane_config_patches").([]interface{})

	controlPlaneConfigPatches := make([]string, len(controlPlaneConfigPatchesRaw))
	for i, raw := range controlPlaneConfigPatchesRaw {
		controlPlaneConfigPatches[i] = raw.(string)
	}

	workerConfigPatchesRaw := d.Get("worker_config_patches").([]interface{})

	workerConfigPatches := make([]string, len(workerConfigPatchesRaw))
	for i, raw := range workerConfigPatchesRaw {
		workerConfigPatches[i] = raw.(string)
	}

	options := []generate.GenOption{}

	configBundle, err := mgmt.GenV1Alpha1Config(
		options,
		clusterName,
		clusterEndpoint,
		constants.DefaultKubernetesVersion,
		configPatches,
		controlPlaneConfigPatches,
		workerConfigPatches,
	)
	if err != nil {
		return err
	}

	d.SetId(clusterName)

	encoderOptions := []encoder.Option{
		encoder.WithComments(encoder.CommentsDisabled),
	}

	controlPlaneConfig, err := configBundle.ControlPlane().EncodeString(encoderOptions...)
	if err != nil {
		return err
	}

	workerConfig, err := configBundle.Worker().EncodeString(encoderOptions...)
	if err != nil {
		return err
	}

	talosConfigBytes, err := yaml.Marshal(configBundle.TalosConfig())
	if err != nil {
		return err
	}

	d.Set("control_plane_machine_configuration", controlPlaneConfig)
	d.Set("worker_machine_configuration", workerConfig)
	d.Set("talosconfig", string(talosConfigBytes))

	return nil
}

func resourceServerRead(d *schema.ResourceData, m interface{}) error {
	return nil
}

func resourceServerDelete(d *schema.ResourceData, m interface{}) error {
	d.SetId("")
	return nil
}

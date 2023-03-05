// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos

import (
	"context"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
)

type pluginProviderServer struct {
	resourceSchemas   map[string]*tfprotov6.Schema
	dataSourceSchemas map[string]*tfprotov6.Schema

	resourceRouter   map[string]func() tfprotov6.ResourceServer
	dataSourceRouter map[string]func() tfprotov6.DataSourceServer
}

type errUnsupportedResource string

func (e errUnsupportedResource) Error() string {
	return "unsupported resource: " + string(e)
}

type errUnsupportedDataSource string

func (e errUnsupportedDataSource) Error() string {
	return "unsupported data source: " + string(e)
}

func (p *pluginProviderServer) GetProviderSchema(ctx context.Context, req *tfprotov6.GetProviderSchemaRequest) (*tfprotov6.GetProviderSchemaResponse, error) {
	return &tfprotov6.GetProviderSchemaResponse{
		DataSourceSchemas: p.dataSourceSchemas,
		ResourceSchemas:   p.resourceSchemas,
	}, nil
}

func (p *pluginProviderServer) ConfigureProvider(ctx context.Context, req *tfprotov6.ConfigureProviderRequest) (*tfprotov6.ConfigureProviderResponse, error) {
	return &tfprotov6.ConfigureProviderResponse{}, nil
}

func (p *pluginProviderServer) StopProvider(ctx context.Context, req *tfprotov6.StopProviderRequest) (*tfprotov6.StopProviderResponse, error) {
	return &tfprotov6.StopProviderResponse{}, nil
}

func (p *pluginProviderServer) ApplyResourceChange(ctx context.Context, req *tfprotov6.ApplyResourceChangeRequest) (*tfprotov6.ApplyResourceChangeResponse, error) {
	res, ok := p.resourceRouter[req.TypeName]
	if !ok {
		return nil, errUnsupportedResource(req.TypeName)
	}

	return res().ApplyResourceChange(ctx, req)
}

func (p *pluginProviderServer) ImportResourceState(ctx context.Context, req *tfprotov6.ImportResourceStateRequest) (*tfprotov6.ImportResourceStateResponse, error) {
	res, ok := p.resourceRouter[req.TypeName]
	if !ok {
		return nil, errUnsupportedResource(req.TypeName)
	}

	return res().ImportResourceState(ctx, req)
}

func (p *pluginProviderServer) ReadDataSource(ctx context.Context, req *tfprotov6.ReadDataSourceRequest) (*tfprotov6.ReadDataSourceResponse, error) {
	ds, ok := p.dataSourceRouter[req.TypeName]
	if !ok {
		return nil, errUnsupportedDataSource(req.TypeName)
	}

	return ds().ReadDataSource(ctx, req)
}

func (p *pluginProviderServer) ReadResource(ctx context.Context, req *tfprotov6.ReadResourceRequest) (*tfprotov6.ReadResourceResponse, error) {
	res, ok := p.resourceRouter[req.TypeName]
	if !ok {
		return nil, errUnsupportedResource(req.TypeName)
	}

	return res().ReadResource(ctx, req)
}

func (p *pluginProviderServer) PlanResourceChange(ctx context.Context, req *tfprotov6.PlanResourceChangeRequest) (*tfprotov6.PlanResourceChangeResponse, error) {
	res, ok := p.resourceRouter[req.TypeName]
	if !ok {
		return nil, errUnsupportedResource(req.TypeName)
	}

	return res().PlanResourceChange(ctx, req)
}

func (p *pluginProviderServer) UpgradeResourceState(ctx context.Context, req *tfprotov6.UpgradeResourceStateRequest) (*tfprotov6.UpgradeResourceStateResponse, error) {
	res, ok := p.resourceRouter[req.TypeName]
	if !ok {
		return nil, errUnsupportedResource(req.TypeName)
	}

	return res().UpgradeResourceState(ctx, req)
}

func (p *pluginProviderServer) ValidateDataResourceConfig(ctx context.Context, req *tfprotov6.ValidateDataResourceConfigRequest) (*tfprotov6.ValidateDataResourceConfigResponse, error) {
	ds, ok := p.dataSourceRouter[req.TypeName]
	if !ok {
		return nil, errUnsupportedDataSource(req.TypeName)
	}

	return ds().ValidateDataResourceConfig(ctx, req)
}

func (p *pluginProviderServer) ValidateProviderConfig(ctx context.Context, req *tfprotov6.ValidateProviderConfigRequest) (*tfprotov6.ValidateProviderConfigResponse, error) {
	return &tfprotov6.ValidateProviderConfigResponse{}, nil
}

func (p *pluginProviderServer) ValidateResourceConfig(ctx context.Context, req *tfprotov6.ValidateResourceConfigRequest) (*tfprotov6.ValidateResourceConfigResponse, error) {
	res, ok := p.resourceRouter[req.TypeName]
	if !ok {
		return nil, errUnsupportedResource(req.TypeName)
	}

	return res().ValidateResourceConfig(ctx, req)
}

func PluginProviderServer() tfprotov6.ProviderServer {
	return &pluginProviderServer{
		resourceSchemas: map[string]*tfprotov6.Schema{
			"talos_machine_configuration_apply": {
				Version: 1,
				Block: &tfprotov6.SchemaBlock{
					Version:     1,
					Description: "Apply a machine configuration to a node.",
					Attributes: []*tfprotov6.SchemaAttribute{
						{
							Name:     "id",
							Type:     tftypes.String,
							Computed: true,
						},
						{
							Name:        "mode",
							Type:        tftypes.String,
							Optional:    true,
							Computed:    true,
							Description: "The mode in which to apply the configuration.",
						},
						{
							Name:        "node",
							Type:        tftypes.String,
							Description: "The node to apply the configuration to.",
							Required:    true,
						},
						{
							Name:        "endpoint",
							Type:        tftypes.String,
							Description: "The endpoint for the talos client. If not specified, the node will be used as the endpoint.",
							Optional:    true,
							Computed:    true,
						},
						{
							Name:        "client_configuration",
							Description: "The client configuration to use when connecting to the talos node",
							Required:    true,
							NestedType: &tfprotov6.SchemaObject{
								Nesting: tfprotov6.SchemaObjectNestingModeSingle,
								Attributes: []*tfprotov6.SchemaAttribute{
									{
										Name:        "ca_certificate",
										Type:        tftypes.String,
										Description: "The CA certificate to use when connecting to the talos node.",
										Required:    true,
									},
									{
										Name:        "client_certificate",
										Type:        tftypes.String,
										Description: "The client certificate to use when connecting to the talos node.",
										Required:    true,
									},
									{
										Name:        "client_key",
										Type:        tftypes.String,
										Description: "The client key to use when connecting to the talos node.",
										Required:    true,
									},
								},
							},
						},
						{
							Name:        "machine_configuration",
							Type:        tftypes.String,
							Description: "The machine configuration to apply to the talos node.",
							Required:    true,
							Sensitive:   true,
						},
						{
							Name:        "machine_configuration_final",
							Type:        tftypes.String,
							Description: "The final machine configuration after applying the patches.",
							Computed:    true,
							Sensitive:   true,
						},
						{
							Name: "config_patches",
							Type: tftypes.List{
								ElementType: tftypes.DynamicPseudoType,
							},
							Description: "The patches to apply to the generated talos configuration",
							Optional:    true,
						},
					},
				},
			},
			"talos_machine_bootstrap": {
				Version: 1,
				Block: &tfprotov6.SchemaBlock{
					Version:     1,
					Description: "Bootstrap etcd on a node.",
					Attributes: []*tfprotov6.SchemaAttribute{
						{
							Name:     "id",
							Type:     tftypes.String,
							Computed: true,
						},
						{
							Name:        "node",
							Type:        tftypes.String,
							Description: "The node to apply the configuration to.",
							Required:    true,
						},
						{
							Name:        "endpoint",
							Type:        tftypes.String,
							Description: "The endpoint for the talos client. If not specified, the node will be used as the endpoint.",
							Optional:    true,
							Computed:    true,
						},
						{
							Name:        "client_configuration",
							Description: "The client configuration to use when connecting to the talos node",
							Required:    true,
							NestedType: &tfprotov6.SchemaObject{
								Nesting: tfprotov6.SchemaObjectNestingModeSingle,
								Attributes: []*tfprotov6.SchemaAttribute{
									{
										Name:        "ca_certificate",
										Type:        tftypes.String,
										Description: "The CA certificate to use when connecting to the talos node.",
										Required:    true,
									},
									{
										Name:        "client_certificate",
										Type:        tftypes.String,
										Description: "The client certificate to use when connecting to the talos node.",
										Required:    true,
									},
									{
										Name:        "client_key",
										Type:        tftypes.String,
										Description: "The client key to use when connecting to the talos node.",
										Required:    true,
									},
								},
							},
						},
					},
				},
			},
		},
		dataSourceSchemas: map[string]*tfprotov6.Schema{
			"talos_machine_configuration": {
				Version: 1,
				Block: &tfprotov6.SchemaBlock{
					Version:     1,
					Description: "Generate a machine configuration for a node.",
					Attributes: []*tfprotov6.SchemaAttribute{
						{
							Name:     "id",
							Type:     tftypes.String,
							Computed: true,
						},
						{
							Name:        "cluster_name",
							Type:        tftypes.String,
							Description: "The name of the talos kubernetes cluster",
							Required:    true,
						},
						{
							Name:        "cluster_endpoint",
							Type:        tftypes.String,
							Description: "The endpoint of the talos kubernetes cluster",
							Required:    true,
						},
						{
							Name:        "machine_secrets",
							Description: "The secrets for the talos kubernetes cluster",
							Required:    true,
							NestedType: &tfprotov6.SchemaObject{
								Nesting: tfprotov6.SchemaObjectNestingModeSingle,
								Attributes: []*tfprotov6.SchemaAttribute{
									{
										Name: "cluster",
										NestedType: &tfprotov6.SchemaObject{
											Nesting: tfprotov6.SchemaObjectNestingModeSingle,
											Attributes: []*tfprotov6.SchemaAttribute{
												{
													Name:        "id",
													Type:        tftypes.String,
													Description: "The cluster id",
													Required:    true,
												},
												{
													Name:        "secret",
													Type:        tftypes.String,
													Description: "The cluster secret",
													Required:    true,
												},
											},
										},
										Description: "The cluster secrets",
										Required:    true,
									},
									{
										Name: "secrets",
										NestedType: &tfprotov6.SchemaObject{
											Nesting: tfprotov6.SchemaObjectNestingModeSingle,
											Attributes: []*tfprotov6.SchemaAttribute{
												{
													Name:        "bootstrap_token",
													Type:        tftypes.String,
													Description: "The bootstrap token for the talos kubernetes cluster",
													Required:    true,
												},
												{
													Name:        "secretbox_encryption_secret",
													Type:        tftypes.String,
													Description: "The secretbox encryption secret for the talos kubernetes cluster",
													Required:    true,
												},
												{
													Name:        "aescbc_encryption_secret",
													Type:        tftypes.String,
													Description: "The aescbc encryption secret for the talos kubernetes cluster",
													Optional:    true,
												},
											},
										},
										Description: "The secrets for the talos kubernetes cluster",
										Required:    true,
									},
									{
										Name: "trustdinfo",
										NestedType: &tfprotov6.SchemaObject{
											Nesting: tfprotov6.SchemaObjectNestingModeSingle,
											Attributes: []*tfprotov6.SchemaAttribute{
												{
													Name:        "token",
													Type:        tftypes.String,
													Description: "The trustd token for the talos cluster",
													Required:    true,
												},
											},
										},
										Description: "The trustd info for the talos cluster",
										Required:    true,
									},
									{
										Name: "certs",
										NestedType: &tfprotov6.SchemaObject{
											Nesting: tfprotov6.SchemaObjectNestingModeSingle,
											Attributes: []*tfprotov6.SchemaAttribute{
												{
													Name:        "etcd",
													NestedType:  certDataWithOptionalCertSchemaObject(true),
													Description: "The etcd certs for the talos cluster",
													Required:    true,
												},
												{
													Name:        "k8s",
													NestedType:  certDataWithOptionalCertSchemaObject(true),
													Description: "The k8s certs for the talos cluster",
													Required:    true,
												},
												{
													Name:        "k8s_aggregator",
													NestedType:  certDataWithOptionalCertSchemaObject(true),
													Description: "The k8s aggregator certs for the talos cluster",
													Required:    true,
												},
												{
													Name:        "k8s_serviceaccount",
													NestedType:  certDataWithOptionalCertSchemaObject(false),
													Description: "The k8s service account certs for the talos cluster",
													Required:    true,
												},
												{
													Name:        "os",
													NestedType:  certDataWithOptionalCertSchemaObject(true),
													Description: "The os certs for the talos cluster",
													Required:    true,
												},
											},
										},
										Description: "The certs for the talos cluster",
										Required:    true,
									},
								},
							},
						},
						{
							Name:        "type",
							Type:        tftypes.String,
							Description: "The type of machine to generate talos configuration for",
							Required:    true,
						},
						{
							Name: "config_patches",
							Type: tftypes.List{
								ElementType: tftypes.DynamicPseudoType,
							},
							Description: "The patches to apply to the generated talos configuration",
							Optional:    true,
						},
						{
							Name:        "kubernetes_version",
							Type:        tftypes.String,
							Description: "The version of kubernetes to use for the talos cluster",
							Optional:    true,
						},
						{
							Name:        "talos_version",
							Type:        tftypes.String,
							Description: "The version of talos machine config to use for the talos cluster",
							Optional:    true,
						},
						{
							Name:        "docs",
							Type:        tftypes.Bool,
							Description: "Whether to generate docs for the talos cluster",
							Optional:    true,
						},
						{
							Name:        "examples",
							Type:        tftypes.Bool,
							Description: "Whether to generate examples for the talos cluster",
							Optional:    true,
						},
						{
							Name:        "machine_configuration",
							Type:        tftypes.String,
							Description: "The generated machine configuration for the talos cluster",
							Computed:    true,
						},
					},
				},
			},
		},
		dataSourceRouter: map[string]func() tfprotov6.DataSourceServer{
			"talos_machine_configuration": NewTalosMachineConfigurationDataSource,
		},
		resourceRouter: map[string]func() tfprotov6.ResourceServer{
			"talos_machine_configuration_apply": NewTalosMachineConfigurationApplyResource,
			"talos_machine_bootstrap":           NewTalosMachineBootstrapResource,
		},
	}
}

func certDataWithOptionalCertSchemaObject(withCert bool) *tfprotov6.SchemaObject {
	attributes := []*tfprotov6.SchemaAttribute{
		{
			Name:        "key",
			Type:        tftypes.String,
			Description: "key data",
			Required:    true,
		},
	}

	if withCert {
		attributes = append(attributes, &tfprotov6.SchemaAttribute{
			Name:        "cert",
			Type:        tftypes.String,
			Description: "certificate data",
			Required:    true,
		})
	}

	return &tfprotov6.SchemaObject{
		Nesting:    tfprotov6.SchemaObjectNestingModeSingle,
		Attributes: attributes,
	}
}

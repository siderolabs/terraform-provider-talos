// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package main

import (
	"context"
	"log"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6/tf6server"
	"github.com/hashicorp/terraform-plugin-mux/tf6muxserver"
	"github.com/siderolabs/terraform-provider-talos/talos"
)

// Provider documentation generation.
//go:generate go run github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs generate --provider-name talos

func main() {
	ctx := context.Background()
	providers := []func() tfprotov6.ProviderServer{
		providerserver.NewProtocol6(talos.New()),
		talos.PluginProviderServer,
	}
	muxServer, err := tf6muxserver.NewMuxServer(ctx, providers...)
	if err != nil {
		log.Fatalln(err.Error())
	}

	// Use the result to start a muxed provider
	if err := tf6server.Serve("registry.terraform.io/siderolabs/talos", muxServer.ProviderServer); err != nil {
		log.Fatalln(err.Error())
	}
}

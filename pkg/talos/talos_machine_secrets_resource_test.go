// This Source Code Form is subject to the terms of the Mozilla Public
// License, v. 2.0. If a copy of the MPL was not distributed with this
// file, You can obtain one at http://mozilla.org/MPL/2.0/.

package talos_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/plancheck"
	"github.com/siderolabs/talos/pkg/machinery/gendata"
	"golang.org/x/mod/semver"

	"github.com/siderolabs/terraform-provider-talos/pkg/talos"
)

func TestAccTalosMachineSecretsResource(t *testing.T) {
	testTime := time.Now()

	resource.ParallelTest(t, resource.TestCase{
		IsUnitTest:               true, // this is a local only resource, so can be unit tested
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// test defaults
			{
				Config: testAccTalosMachineSecretsResourceConfig(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "id", "machine_secrets"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "talos_version", semver.MajorMinor(gendata.VersionTag)),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.id"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.secret"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.bootstrap_token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.secretbox_encryption_secret"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.trustdinfo.token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_serviceaccount.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.client_key"),
					resource.TestCheckNoResourceAttr("talos_machine_secrets.this", "machine_secrets.secrets.aescbc_encryption_secret"),
				),
			},
			// test talosconfig regeneration
			{
				Config: testAccTalosMachineSecretsResourceConfig(""),
				PreConfig: func() {
					talos.OverridableTimeFunc = func() time.Time {
						return testTime.AddDate(0, 12, 5)
					}
				},
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "id", "machine_secrets"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "talos_version", semver.MajorMinor(gendata.VersionTag)),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.id"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.secret"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.bootstrap_token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.secretbox_encryption_secret"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.trustdinfo.token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_serviceaccount.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.client_key"),
					resource.TestCheckNoResourceAttr("talos_machine_secrets.this", "machine_secrets.secrets.aescbc_encryption_secret"),
				),
			},
			// test that setting the talos_version to the same value does not cause a diff
			{
				Config: testAccTalosMachineSecretsResourceConfig(semver.MajorMinor(gendata.VersionTag)),
				PreConfig: func() {
					talos.OverridableTimeFunc = func() time.Time {
						return testTime
					}
				},
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectEmptyPlan(),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "id", "machine_secrets"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "talos_version", semver.MajorMinor(gendata.VersionTag)),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.id"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.secret"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.bootstrap_token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.secretbox_encryption_secret"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.trustdinfo.token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_serviceaccount.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.client_key"),
					resource.TestCheckNoResourceAttr("talos_machine_secrets.this", "machine_secrets.secrets.aescbc_encryption_secret"),
				),
			},
			// test that setting the talos_version to a lower version causes a diff and requires replacement
			// also test that the aescbc_encryption_secret is set
			{ //nolint:dupl
				Config: testAccTalosMachineSecretsResourceConfig("v1.2.0"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectNonEmptyPlan(),
						plancheck.ExpectResourceAction("talos_machine_secrets.this", plancheck.ResourceActionReplace),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "id", "machine_secrets"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "talos_version", "v1.2.0"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.id"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.secret"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.bootstrap_token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.trustdinfo.token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_serviceaccount.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.client_key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.aescbc_encryption_secret"),
				),
			},
			// test that setting the talos_version to a higher version does not cause a diff and requires no replacement
			// also test that the aescbc_encryption_secret is still set when upgrading
			{ //nolint:dupl
				Config: testAccTalosMachineSecretsResourceConfig("1.4"),
				ConfigPlanChecks: resource.ConfigPlanChecks{
					PreApply: []plancheck.PlanCheck{
						plancheck.ExpectNonEmptyPlan(),
						plancheck.ExpectResourceAction("talos_machine_secrets.this", plancheck.ResourceActionUpdate),
					},
				},
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "id", "machine_secrets"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "talos_version", "1.4"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.id"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.secret"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.bootstrap_token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.trustdinfo.token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_serviceaccount.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.ca_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.client_key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.aescbc_encryption_secret"),
				),
			},
		},
	})

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true, // this is a local only resource, so can be unit tested
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// test importing from a secrets.yaml file
			{
				Config:             testAccTalosMachineSecretsResourceConfig(""),
				ResourceName:       "talos_machine_secrets.this",
				ImportState:        true,
				ImportStatePersist: true,
				ImportStateId:      "testdata/secrets.yaml",
			},
			// verify there are no changes after import
			{
				Config:   testAccTalosMachineSecretsResourceConfig(""),
				PlanOnly: true,
			},
			// verify state is correct after import
			{
				Config: testAccTalosMachineSecretsResourceConfig(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "id", "machine_secrets"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "talos_version", "v1.3"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.cluster.id", "_u8NZvwQ9ObtEN7iTzc-OEpk20K-rnO3FNcjvVEQ84Q="),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.cluster.secret", "UnZE8oq6qPNI8tuw+WF3PGi2Zba0RQuit/aJTflOau8="),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.secrets.bootstrap_token", "m2wfba.pcyzhp6rf6pubqtk"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.secrets.secretbox_encryption_secret", "avDR6jwn4iYS6sTOH689P2UcNUlh3UsuO+FaOKI2hls="),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.trustdinfo.token", "s5lcto.f2ythdlx6avcsny9"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.etcd.cert", "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJmakNDQVNTZ0F3SUJBZ0lSQUt6KzNQbkZYWHNTdXNQb3RLTnVabG93Q2dZSUtvWkl6ajBFQXdJd0R6RU4KTUFzR0ExVUVDaE1FWlhSalpEQWVGdzB5TXpBME1ERXhORE01TURaYUZ3MHpNekF6TWpreE5ETTVNRFphTUE4eApEVEFMQmdOVkJBb1RCR1YwWTJRd1dUQVRCZ2NxaGtqT1BRSUJCZ2dxaGtqT1BRTUJCd05DQUFTbGFaU0p1UnhMCit4NDdYelY5OWIwaEd1dmdybGZrUnFEdStVUWlrSFJlRCtFL3VZUXprc1ArZTFLMVBUcFJVTG45ZEJYY21jd1AKazY2UXRCN09mQ0pEbzJFd1h6QU9CZ05WSFE4QkFmOEVCQU1DQW9Rd0hRWURWUjBsQkJZd0ZBWUlLd1lCQlFVSApBd0VHQ0NzR0FRVUZCd01DTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3SFFZRFZSME9CQllFRk5kVHVVdmduZXcwCkJSa0dZRnJueWNpWFZjUTdNQW9HQ0NxR1NNNDlCQU1DQTBnQU1FVUNJUURJZGVqQ3ppL25xb3h0eUp3QnROTEYKTDE3UWdqQjNzMFoySUV5ZDZPTE51UUlnY0lHcWRWemJKQ3dXSkZXSWxnWWVyUGZvZzdzUjFxMWdKV25ST3p4UApBakE9Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"),                        //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.etcd.key", "LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1IY0NBUUVFSUoxWVI2ck9pd3N2TmRWZFErVnpiQ1hlRlBEbGJ1cDVUM1ZidnJrVDM2M1NvQW9HQ0NxR1NNNDkKQXdFSG9VUURRZ0FFcFdtVWlia2NTL3NlTzE4MWZmVzlJUnJyNEs1WDVFYWc3dmxFSXBCMFhnL2hQN21FTTVMRAovbnRTdFQwNlVWQzUvWFFWM0puTUQ1T3VrTFFlem53aVF3PT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo="),                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.k8s.cert", "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJpakNDQVMrZ0F3SUJBZ0lRSE5COFFlVjRtazlUR3k4amtTUGtYVEFLQmdncWhrak9QUVFEQWpBVk1STXcKRVFZRFZRUUtFd3ByZFdKbGNtNWxkR1Z6TUI0WERUSXpNRFF3TVRFME16a3dObG9YRFRNek1ETXlPVEUwTXprdwpObG93RlRFVE1CRUdBMVVFQ2hNS2EzVmlaWEp1WlhSbGN6QlpNQk1HQnlxR1NNNDlBZ0VHQ0NxR1NNNDlBd0VICkEwSUFCQVBKQjEwL3RnVFlkdVhrS0h6SVFEZDNLbW54MithbmFXcGN3RUlhWlhMbnNsRHRycGU3ancwaXRVK1oKa2w2eEd5STk4M0FpRkxWc004cHhra3RoZ2tTallUQmZNQTRHQTFVZER3RUIvd1FFQXdJQ2hEQWRCZ05WSFNVRQpGakFVQmdnckJnRUZCUWNEQVFZSUt3WUJCUVVIQXdJd0R3WURWUjBUQVFIL0JBVXdBd0VCL3pBZEJnTlZIUTRFCkZnUVU1K0hGd29PbTdURWhpcm1DTGt2SllyRmRQc293Q2dZSUtvWkl6ajBFQXdJRFNRQXdSZ0loQU1WZFNVUEUKcGlNWkpkNHNYV1pvdzdLb0djWWhPb1NtcDJkaUZEdUEzMjcyQWlFQTRuamcwN2ZhVEFGck1SUGF4SmFIMVhsdQpKTHBPMTh0bHZhVlVFMW5XY2xnPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="), //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.k8s.key", "LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1IY0NBUUVFSUZJQ0crOUR1Wm9YU0xiVjRWK1VoZkdRUWV5OUJ3dWEwZUx6M0hTU1hyMmhvQW9HQ0NxR1NNNDkKQXdFSG9VUURRZ0FFQThrSFhUKzJCTmgyNWVRb2ZNaEFOM2NxYWZIYjVxZHBhbHpBUWhwbGN1ZXlVTzJ1bDd1UApEU0sxVDVtU1hyRWJJajN6Y0NJVXRXd3p5bkdTUzJHQ1JBPT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo="),                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.cert", "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJZVENDQVFhZ0F3SUJBZ0lSQUtDY3hEVUM4UG9GK3pSajJWVlJLWTR3Q2dZSUtvWkl6ajBFQXdJd0FEQWUKRncweU16QTBNREV4TkRNNU1EWmFGdzB6TXpBek1qa3hORE01TURaYU1BQXdXVEFUQmdjcWhrak9QUUlCQmdncQpoa2pPUFFNQkJ3TkNBQVFPUjZKcTl1ekp3RWNFZnowc1dKdERVejhwNUpseHRJZkJjTTMzRFJ5cDhCSDZGQTFCCjJiUkx5ZTBsU3p6TXFmdVpFblhxOE9qazdobklGSUdsNlVNaW8yRXdYekFPQmdOVkhROEJBZjhFQkFNQ0FvUXcKSFFZRFZSMGxCQll3RkFZSUt3WUJCUVVIQXdFR0NDc0dBUVVGQndNQ01BOEdBMVVkRXdFQi93UUZNQU1CQWY4dwpIUVlEVlIwT0JCWUVGQUpPaUFGNTRhUXRPaUpzdFU2U2Z1QWpGOFJwTUFvR0NDcUdTTTQ5QkFNQ0Ewa0FNRVlDCklRREdPOUhXVXNaZlozRUdJTEdGRjJwUDIwUGNjSDZMV2hZSjg1eHB2RDBOSGdJaEFOOHFpVElXYVF2dUs3M1cKVENwT2g5TU1PS2o1YmFNcEh6M2FJOGVMZHBiVAotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="),                                                                  //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.key", "LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1IY0NBUUVFSU4rRDdGWGJoUTg4c1d6eEJPSXZtMVJVVVBtT25VeFBCRGlvR21mL1dnMjlvQW9HQ0NxR1NNNDkKQXdFSG9VUURRZ0FFRGtlaWF2YnN5Y0JIQkg4OUxGaWJRMU0vS2VTWmNiU0h3WEROOXcwY3FmQVIraFFOUWRtMApTOG50SlVzOHpLbjdtUkoxNnZEbzVPNFp5QlNCcGVsRElnPT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo="),                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.k8s_serviceaccount.key", "LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1IY0NBUUVFSUVabmsrN2djSXBLdC8vM2lrQ1dLZm5SNnFjeWY5bHVyK1lGbzQyTlhVVVlvQW9HQ0NxR1NNNDkKQXdFSG9VUURRZ0FFeG9rOTBEbnI2MGRYQ3BhQmxUMnVJUnRjdUJPLytCVjZEajVPaWYvTGVwSS95NktlQ2lRTApwRGZxenNHQW5zQXkrVHI3SWNxSUlROGZGaXRtK0t5emJBPT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo="),                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.os.cert", "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJQekNCOHFBREFnRUNBaEVBM0E0TEZ3cDVTZG5wTXVmYlp3NHVpekFGQmdNclpYQXdFREVPTUF3R0ExVUUKQ2hNRmRHRnNiM013SGhjTk1qTXdOREF4TVRRek9UQTJXaGNOTXpNd016STVNVFF6T1RBMldqQVFNUTR3REFZRApWUVFLRXdWMFlXeHZjekFxTUFVR0F5dGxjQU1oQUZ2TWUyeHFzSk9WemkvY0xLNXVXVzU2VmZZK29nWlYvQVowCmxlTFdtTWl1bzJFd1h6QU9CZ05WSFE4QkFmOEVCQU1DQW9Rd0hRWURWUjBsQkJZd0ZBWUlLd1lCQlFVSEF3RUcKQ0NzR0FRVUZCd01DTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3SFFZRFZSME9CQllFRkxqK3VablllVFFUdEdmUgpOeVVQYWh6dzNkdkhNQVVHQXl0bGNBTkJBQVg4cVhJNm4ydlk3ZGxnZGtxckUvN25ua2kwTzFtVERDL3dBamlwCmpaemY5QmhocEdRUXFYSkxHdlhJTnRDaXN5KzQrcTVtOUVjUUpMMXF4UWdOdndnPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="),                                                                                                                                          //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.os.key", "LS0tLS1CRUdJTiBFRDI1NTE5IFBSSVZBVEUgS0VZLS0tLS0KTUM0Q0FRQXdCUVlESzJWd0JDSUVJSlVlQmxic3hhMW0vR1B0NTZCeVIvZ1Z3YWRzVmdkc3pXZEh4MWZiZ0c4VwotLS0tLUVORCBFRDI1NTE5IFBSSVZBVEUgS0VZLS0tLS0K"),                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "client_configuration.ca_certificate", "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJQekNCOHFBREFnRUNBaEVBM0E0TEZ3cDVTZG5wTXVmYlp3NHVpekFGQmdNclpYQXdFREVPTUF3R0ExVUUKQ2hNRmRHRnNiM013SGhjTk1qTXdOREF4TVRRek9UQTJXaGNOTXpNd016STVNVFF6T1RBMldqQVFNUTR3REFZRApWUVFLRXdWMFlXeHZjekFxTUFVR0F5dGxjQU1oQUZ2TWUyeHFzSk9WemkvY0xLNXVXVzU2VmZZK29nWlYvQVowCmxlTFdtTWl1bzJFd1h6QU9CZ05WSFE4QkFmOEVCQU1DQW9Rd0hRWURWUjBsQkJZd0ZBWUlLd1lCQlFVSEF3RUcKQ0NzR0FRVUZCd01DTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3SFFZRFZSME9CQllFRkxqK3VablllVFFUdEdmUgpOeVVQYWh6dzNkdkhNQVVHQXl0bGNBTkJBQVg4cVhJNm4ydlk3ZGxnZGtxckUvN25ua2kwTzFtVERDL3dBamlwCmpaemY5QmhocEdRUXFYSkxHdlhJTnRDaXN5KzQrcTVtOUVjUUpMMXF4UWdOdndnPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="),                                                                                                                                    //nolint:lll
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.client_key"),
					resource.TestCheckNoResourceAttr("talos_machine_secrets.this", "machine_secrets.secrets.aescbc_encryption_secret"),
				),
			},
		},
	})

	resource.Test(t, resource.TestCase{
		IsUnitTest:               true, // this is a local only resource, so can be unit tested
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			// test importing from a v1.2 secrets file
			{
				Config:             testAccTalosMachineSecretsResourceConfig(""),
				ResourceName:       "talos_machine_secrets.this",
				ImportState:        true,
				ImportStatePersist: true,
				ImportStateId:      "testdata/secretsv1.2.yaml",
			},
			// verify that there are no diffs
			{
				Config:   testAccTalosMachineSecretsResourceConfig(""),
				PlanOnly: true,
			},
			// verify state is correct after import
			{
				Config: testAccTalosMachineSecretsResourceConfig(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "id", "machine_secrets"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "talos_version", "v1.2"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.cluster.id", "q_I385nl7MWqU1UpW224rQyZW4TWd_WmnxsA2MQLsl8="),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.cluster.secret", "1szT7qMuensSCcSVRtnFsG0pbXMLMSZ8r5wu/41aJBc="),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.secrets.bootstrap_token", "5co9z6.qnnjtotc5ffntt62"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.secrets.secretbox_encryption_secret", ""),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.trustdinfo.token", "o2q4ek.ofdeihu3li44x7lr"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.etcd.cert", "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJmakNDQVNTZ0F3SUJBZ0lSQU8rYmlXSlFJdHZXZUZ0UUNEVDhwRXd3Q2dZSUtvWkl6ajBFQXdJd0R6RU4KTUFzR0ExVUVDaE1FWlhSalpEQWVGdzB5TXpBME1ERXhOVFF4TURSYUZ3MHpNekF6TWpreE5UUXhNRFJhTUE4eApEVEFMQmdOVkJBb1RCR1YwWTJRd1dUQVRCZ2NxaGtqT1BRSUJCZ2dxaGtqT1BRTUJCd05DQUFReHRnaFlZV1AvCngzWGFBM1RPVXd4Y1AySlh2QVYzdllaS2I1SENKK0E2M2dieE50dW5Gcm03NW8rK0ZQQndYdUtZUmMrU09pTXEKdWY5bjdkZmY4cUJKbzJFd1h6QU9CZ05WSFE4QkFmOEVCQU1DQW9Rd0hRWURWUjBsQkJZd0ZBWUlLd1lCQlFVSApBd0VHQ0NzR0FRVUZCd01DTUE4R0ExVWRFd0VCL3dRRk1BTUJBZjh3SFFZRFZSME9CQllFRkU2VUVneitoOSsvCnZYdnNod1d2bHJvQkh6SlFNQW9HQ0NxR1NNNDlCQU1DQTBnQU1FVUNJUUQ3YTFzMy91cmRybFlxaXJxTmN6aTIKTm5qNVFCdFVmczFNYkNqNTdkYXR3d0lnZlJYdEIyWVZMUy9OMFVQTDkxekhITCtLM09EaEZ2M1dsc0gwMlpmdgpMNHc9Ci0tLS0tRU5EIENFUlRJRklDQVRFLS0tLS0K"),                        //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.etcd.key", "LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1IY0NBUUVFSUJDNUpZQmgzWWQ2NnJwWkcvTHZEMWt4SFRvWTA4QnZsQkpMcy9aZXR4NldvQW9HQ0NxR1NNNDkKQXdFSG9VUURRZ0FFTWJZSVdHRmovOGQxMmdOMHpsTU1YRDlpVjd3RmQ3MkdTbStSd2lmZ090NEc4VGJicHhhNQp1K2FQdmhUd2NGN2ltRVhQa2pvaktybi9aKzNYMy9LZ1NRPT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo="),                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.k8s.cert", "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJpVENDQVMrZ0F3SUJBZ0lRVW9hWGkyZGxUWHk1alNBUVdvSUZOVEFLQmdncWhrak9QUVFEQWpBVk1STXcKRVFZRFZRUUtFd3ByZFdKbGNtNWxkR1Z6TUI0WERUSXpNRFF3TVRFMU5ERXdORm9YRFRNek1ETXlPVEUxTkRFdwpORm93RlRFVE1CRUdBMVVFQ2hNS2EzVmlaWEp1WlhSbGN6QlpNQk1HQnlxR1NNNDlBZ0VHQ0NxR1NNNDlBd0VICkEwSUFCRWJieTQreTdIWmZUcVNNaDRtMWx3a3E3THE5WUtWdVhmV3BJLzZ4K1orZC9uUFJFNFA0eGhNUklKL3oKZHl4TDJxTGNSOUcxV3pjVWJBRVYrMjliUWNDallUQmZNQTRHQTFVZER3RUIvd1FFQXdJQ2hEQWRCZ05WSFNVRQpGakFVQmdnckJnRUZCUWNEQVFZSUt3WUJCUVVIQXdJd0R3WURWUjBUQVFIL0JBVXdBd0VCL3pBZEJnTlZIUTRFCkZnUVUzRG8xb2MzM054dWhYRTJ4M2NvbHM3a2dWWmN3Q2dZSUtvWkl6ajBFQXdJRFNBQXdSUUloQU9pb3RHR1MKNnJLdWFoT2twMlQ5NjRzSUhhdmx3bUJqZ0ljVkI1dTFPV2RsQWlBRXZKUUVTRGNBZWlmVm1seG1pdWVDMHl4SQowODBqY2FxUjdUVWNjaHhrSnc9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="), //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.k8s.key", "LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1IY0NBUUVFSUx5bm9FaURjeG1NL3ZlL1B1R0YwWFNjeEgydG10SGZSdVU0SkVvejBGRmtvQW9HQ0NxR1NNNDkKQXdFSG9VUURRZ0FFUnR2TGo3THNkbDlPcEl5SGliV1hDU3JzdXIxZ3BXNWQ5YWtqL3JINW41MytjOUVUZy9qRwpFeEVnbi9OM0xFdmFvdHhIMGJWYk54UnNBUlg3YjF0QndBPT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo="),                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.cert", "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJZRENDQVFhZ0F3SUJBZ0lSQUp5THJmLzk5aE00a1ZNWTJraFd4TjR3Q2dZSUtvWkl6ajBFQXdJd0FEQWUKRncweU16QTBNREV4TlRReE1EUmFGdzB6TXpBek1qa3hOVFF4TURSYU1BQXdXVEFUQmdjcWhrak9QUUlCQmdncQpoa2pPUFFNQkJ3TkNBQVFZTTBWWUVZWTVOaURlNHp6Z21mYlJSdDRNalRmOUlyeUVVcms1SjNPRFQzYkFpanVDCmd6eWpmTkZIbFlXbm9PcWZBeGpjVVVsQkU2L2xuRmdiMzNwUW8yRXdYekFPQmdOVkhROEJBZjhFQkFNQ0FvUXcKSFFZRFZSMGxCQll3RkFZSUt3WUJCUVVIQXdFR0NDc0dBUVVGQndNQ01BOEdBMVVkRXdFQi93UUZNQU1CQWY4dwpIUVlEVlIwT0JCWUVGTHBKUEhhRGw1UjY4NjlDNEVyVXF5WHhBeUpWTUFvR0NDcUdTTTQ5QkFNQ0EwZ0FNRVVDCklFc0VlOElSNEwvK3phMC9CbzlUNGRKbERpQ3VDK1BSd1JueXE4OEE5dFUvQWlFQTBGUXNJcGk5V1ZiU2krODQKQkVCaGRWTkpmQUVUYTZVQ2UrTWFsNUJRUjlrPQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="),                                                                  //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.key", "LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1IY0NBUUVFSUMxSDViQUY5RkJWQzVIQjZ2dzJwL1FRUFkvWEVjdzhaTUJ0ZDZFTmw3cXFvQW9HQ0NxR1NNNDkKQXdFSG9VUURRZ0FFR0RORldCR0dPVFlnM3VNODRKbjIwVWJlREkwMy9TSzhoRks1T1NkemcwOTJ3SW83Z29NOApvM3pSUjVXRnA2RHFud01ZM0ZGSlFST3Y1WnhZRzk5NlVBPT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo="),                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.k8s_serviceaccount.key", "LS0tLS1CRUdJTiBFQyBQUklWQVRFIEtFWS0tLS0tCk1IY0NBUUVFSUxHRjlhcmp4SEdyMExMbTdMTjg0Ympjbml2RWpGSkxlUFNvVGhZUS9maWdvQW9HQ0NxR1NNNDkKQXdFSG9VUURRZ0FFNjRuQVlld3hqeVkrVkY4MDZ5WU5iU3pnNEl4cFh6TW1hMW93b3FjbDc5elZtMkRsbHcxUApYR3FTZ0hpWUxwcjZ1ZU5OeSswcXdOdklCU3RKSXZLVzNnPT0KLS0tLS1FTkQgRUMgUFJJVkFURSBLRVktLS0tLQo="),                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.os.cert", "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJQakNCOGFBREFnRUNBaEIzcXY0bDNhTStSaGFsWmdHOGowdDNNQVVHQXl0bGNEQVFNUTR3REFZRFZRUUsKRXdWMFlXeHZjekFlRncweU16QTBNREV4TlRReE1EUmFGdzB6TXpBek1qa3hOVFF4TURSYU1CQXhEakFNQmdOVgpCQW9UQlhSaGJHOXpNQ293QlFZREsyVndBeUVBWk1TT1d4aHF4UThEbHdUVmszM2xRN09ydDAvOTE5b0JXTVpUCmRSU3Q4SGFqWVRCZk1BNEdBMVVkRHdFQi93UUVBd0lDaERBZEJnTlZIU1VFRmpBVUJnZ3JCZ0VGQlFjREFRWUkKS3dZQkJRVUhBd0l3RHdZRFZSMFRBUUgvQkFVd0F3RUIvekFkQmdOVkhRNEVGZ1FVYkhDeHcyOTd4RHc3Tjh0SQpDQTJTUDc2K3Q5OHdCUVlESzJWd0EwRUFrcnQ1UEVnZ2JZNFFlYnNIa2lDTmZlMFpZNlE1UmZhVm52TVRxOE1lCnRhSktTQ1NPYTljczh2dXVDMnl2QmNSU0hPWldocG9WaW05bXhEaVc3TDZTQXc9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="),                                                                                                                                          //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.certs.os.key", "LS0tLS1CRUdJTiBFRDI1NTE5IFBSSVZBVEUgS0VZLS0tLS0KTUM0Q0FRQXdCUVlESzJWd0JDSUVJRjdtNmJKWHNOd3F4ejFMaXRnVlFJSEx5WDJab1hadW85UTNEZjRGSThWaAotLS0tLUVORCBFRDI1NTE5IFBSSVZBVEUgS0VZLS0tLS0K"),                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           //nolint:lll
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "client_configuration.ca_certificate", "LS0tLS1CRUdJTiBDRVJUSUZJQ0FURS0tLS0tCk1JSUJQakNCOGFBREFnRUNBaEIzcXY0bDNhTStSaGFsWmdHOGowdDNNQVVHQXl0bGNEQVFNUTR3REFZRFZRUUsKRXdWMFlXeHZjekFlRncweU16QTBNREV4TlRReE1EUmFGdzB6TXpBek1qa3hOVFF4TURSYU1CQXhEakFNQmdOVgpCQW9UQlhSaGJHOXpNQ293QlFZREsyVndBeUVBWk1TT1d4aHF4UThEbHdUVmszM2xRN09ydDAvOTE5b0JXTVpUCmRSU3Q4SGFqWVRCZk1BNEdBMVVkRHdFQi93UUVBd0lDaERBZEJnTlZIU1VFRmpBVUJnZ3JCZ0VGQlFjREFRWUkKS3dZQkJRVUhBd0l3RHdZRFZSMFRBUUgvQkFVd0F3RUIvekFkQmdOVkhRNEVGZ1FVYkhDeHcyOTd4RHc3Tjh0SQpDQTJTUDc2K3Q5OHdCUVlESzJWd0EwRUFrcnQ1UEVnZ2JZNFFlYnNIa2lDTmZlMFpZNlE1UmZhVm52TVRxOE1lCnRhSktTQ1NPYTljczh2dXVDMnl2QmNSU0hPWldocG9WaW05bXhEaVc3TDZTQXc9PQotLS0tLUVORCBDRVJUSUZJQ0FURS0tLS0tCg=="),                                                                                                                                    //nolint:lll
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.client_certificate"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "client_configuration.client_key"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "machine_secrets.secrets.aescbc_encryption_secret", "hLrjDIpZ8gSGwejFfnUnjOrn9PQ7Bj3yq/ggAgD9AHA="),
				),
			},
		},
	})
}

func TestAccTalosMachineSecretsResourceUpgrade1(t *testing.T) {
	resource.ParallelTest(t, resource.TestCase{
		IsUnitTest: true, // this is a local only resource, so can be unit tested
		Steps: []resource.TestStep{
			// create talos_machine_secrets resource with talos version 0.1.2
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"talos": {
						VersionConstraint: "=0.1.2",
						Source:            "siderolabs/talos",
					},
				},
				Config: testAccTalosMachineSecretsResourceConfig(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "id"),
					resource.TestCheckNoResourceAttr("talos_machine_secrets.this", "talos_version"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets"),
				),
			},
			// verify the new state is compatible with the latest version of the provider
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   testAccTalosMachineSecretsResourceConfig(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "id", "machine_secrets"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "talos_version", "v1.3"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.id"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.secret"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.bootstrap_token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.secretbox_encryption_secret"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.trustdinfo.token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_serviceaccount.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.key"),
					resource.TestCheckNoResourceAttr("talos_machine_secrets.this", "machine_secrets.secrets.aescbc_encryption_secret"),
				),
			},
		},
	})
}

func TestAccTalosMachineSecretsResourceUpgrade2(t *testing.T) { //nolint:dupl
	resource.ParallelTest(t, resource.TestCase{
		IsUnitTest: true, // this is a local only resource, so can be unit tested
		Steps: []resource.TestStep{
			// create talos_machine_secrets resource with talos version 0.1.2
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"talos": {
						VersionConstraint: "=0.1.2",
						Source:            "siderolabs/talos",
					},
				},
				Config: testAccTalosMachineSecretsResourceConfig("v1.2"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "id"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "talos_version", "v1.2"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets"),
				),
			},
			// verify the new state is compatible with the latest version of the provider
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   testAccTalosMachineSecretsResourceConfig(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "id", "machine_secrets"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "talos_version", "v1.2"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.id"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.secret"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.bootstrap_token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.trustdinfo.token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_serviceaccount.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.aescbc_encryption_secret"),
				),
			},
		},
	})
}

func TestAccTalosMachineSecretsResourceUpgrade3(t *testing.T) { //nolint:dupl
	resource.ParallelTest(t, resource.TestCase{
		IsUnitTest: true, // this is a local only resource, so can be unit tested
		Steps: []resource.TestStep{
			// create talos_machine_secrets resource with talos version 0.1.2
			{
				ExternalProviders: map[string]resource.ExternalProvider{
					"talos": {
						VersionConstraint: "=0.1.2",
						Source:            "siderolabs/talos",
					},
				},
				Config: testAccTalosMachineSecretsResourceConfig("v1.1"),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "id"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "talos_version", "v1.1"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets"),
				),
			},
			// verify the new state is compatible with the latest version of the provider
			{
				ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
				Config:                   testAccTalosMachineSecretsResourceConfig(""),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "id", "machine_secrets"),
					resource.TestCheckResourceAttr("talos_machine_secrets.this", "talos_version", "v1.2"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.id"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.cluster.secret"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.bootstrap_token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.trustdinfo.token"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.etcd.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_aggregator.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.k8s_serviceaccount.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.cert"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.certs.os.key"),
					resource.TestCheckResourceAttrSet("talos_machine_secrets.this", "machine_secrets.secrets.aescbc_encryption_secret"),
				),
			},
		},
	})
}

func testAccTalosMachineSecretsResourceConfig(talosConfigVersion string) string {
	if talosConfigVersion != "" {
		return fmt.Sprintf(`
resource "talos_machine_secrets" "this" {
	talos_version = "%s"
}
`, talosConfigVersion)
	}

	return `
resource "talos_machine_secrets" "this" {}
`
}

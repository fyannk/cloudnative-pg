/*
Copyright © contributors to CloudNativePG, established as
CloudNativePG a Series of LF Projects, LLC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

SPDX-License-Identifier: Apache-2.0
*/

package controller

import (
	"os"
	"path/filepath"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/certs"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Goal: the webhook server gets the right certificate files, and a bad
// WEBHOOK_CERT_DIR stops the start before the manager exists
var _ = Describe("Webhook server options", func() {
	It("uses the operator PKI directory and the TLS Secret names by default", func() {
		options, err := buildWebhookOptions(9443, &configuration.Data{})
		Expect(err).ToNot(HaveOccurred())
		Expect(options.Port).To(Equal(9443))
		Expect(options.CertDir).To(Equal(certs.DefaultWebhookCertDir))
		Expect(options.CertName).To(Equal("tls.crt"))
		Expect(options.KeyName).To(Equal("tls.key"))
	})

	It("uses the externally provided certificate under the convention found", func() {
		dir := GinkgoT().TempDir()
		for _, name := range []string{"tls.crt", "tls.key", "ca.crt"} {
			Expect(os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600)).To(Succeed())
		}

		options, err := buildWebhookOptions(9443, &configuration.Data{WebhookCertDir: dir})
		Expect(err).ToNot(HaveOccurred())
		Expect(options.CertDir).To(Equal(dir))
		Expect(options.CertName).To(Equal("tls.crt"))
		Expect(options.KeyName).To(Equal("tls.key"))
	})

	It("keeps the OLM names when OLM provides the certificate", func() {
		dir := GinkgoT().TempDir()
		for _, name := range []string{"apiserver.crt", "apiserver.key"} {
			Expect(os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600)).To(Succeed())
		}

		options, err := buildWebhookOptions(9443, &configuration.Data{WebhookCertDir: dir})
		Expect(err).ToNot(HaveOccurred())
		Expect(options.CertName).To(Equal("apiserver.crt"))
		Expect(options.KeyName).To(Equal("apiserver.key"))
	})

	It("refuses to start when the directory holds no certificate pair", func() {
		dir := GinkgoT().TempDir()
		_, err := buildWebhookOptions(9443, &configuration.Data{WebhookCertDir: dir})
		Expect(err).To(MatchError(ContainSubstring("no webhook certificate pair")))
	})
})

var _ = Describe("Operator identity", func() {
	It("accepts an absent Deployment name", func() {
		Expect(validateOperatorIdentity(&configuration.Data{})).To(Succeed())
	})

	It("accepts a Deployment name that can name Kubernetes objects", func() {
		Expect(validateOperatorIdentity(&configuration.Data{OperatorDeploymentName: "cnpg-toto"})).To(Succeed())
	})

	It("refuses a Deployment name that cannot name Kubernetes objects", func() {
		err := validateOperatorIdentity(&configuration.Data{OperatorDeploymentName: "CNPG_toto"})
		Expect(err).To(MatchError(ContainSubstring("OPERATOR_DEPLOYMENT_NAME")))
	})
})

var _ = Describe("Operator status identity", func() {
	It("is refused with the identity when the mode is unknown", func() {
		err := validateOperatorIdentity(&configuration.Data{OperatorStatusIdentity: "verbose"})
		Expect(err).To(MatchError(ContainSubstring("OPERATOR_STATUS_IDENTITY")))
	})
})

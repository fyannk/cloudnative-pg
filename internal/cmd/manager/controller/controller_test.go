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
	"context"
	"os"
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/certs"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/versions"

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

var _ = Describe("Operator user agent", func() {
	It("is the default one when the Deployment name is not reported", func() {
		Expect(operatorUserAgent(&configuration.Data{
			OperatorDeploymentName: "cnpg-toto",
			OperatorStatusIdentity: configuration.OperatorStatusIdentityVersion,
		})).To(BeEmpty())
		Expect(operatorUserAgent(&configuration.Data{
			OperatorDeploymentName: "cnpg-toto",
			OperatorStatusIdentity: configuration.OperatorStatusIdentityOff,
		})).To(BeEmpty())
	})

	It("is the default one when the Deployment name is not known", func() {
		Expect(operatorUserAgent(&configuration.Data{
			OperatorStatusIdentity: configuration.OperatorStatusIdentityFull,
		})).To(BeEmpty())
	})

	It("starts with the Deployment name, the field manager, when the name is reported", func() {
		for _, mode := range []string{
			configuration.OperatorStatusIdentityName, configuration.OperatorStatusIdentityFull,
		} {
			userAgent := operatorUserAgent(&configuration.Data{
				OperatorDeploymentName: "cnpg-toto",
				OperatorStatusIdentity: mode,
			})
			Expect(userAgent).To(HavePrefix("cnpg-toto/" + versions.Version + " ("))
			Expect(userAgent).To(ContainSubstring("cloudnative-pg/"))
		}
	})
})

var _ = Describe("Operator status identity", func() {
	It("is refused with the identity when the mode is unknown", func() {
		err := validateOperatorIdentity(&configuration.Data{OperatorStatusIdentity: "verbose"})
		Expect(err).To(MatchError(ContainSubstring("OPERATOR_STATUS_IDENTITY")))
	})
})

// NODE_STATUS_UNAVAILABLE_POLICY: an unknown value stops the start when the
// configuration is loaded, fail stops it once the Node API is found missing
var _ = Describe("The policy when no node state is read", func() {
	It("refuses an unknown policy when the configuration is loaded", func() {
		conf := configuration.NewConfiguration()
		conf.NodeStatusUnavailablePolicy = "ignore"
		Expect(checkLoadedConfiguration(conf)).To(MatchError(ContainSubstring("NODE_STATUS_UNAVAILABLE_POLICY")))
	})

	It("refuses to start with fail when the Node API is not accessible", func() {
		conf := configuration.NewConfiguration()
		conf.APINodeEnabled = false
		conf.NodeStatusUnavailablePolicy = configuration.NodeStatusUnavailablePolicyFail
		Expect(resolveNodeStatusSource(conf)).To(MatchError(ContainSubstring("NODE_STATUS_UNAVAILABLE_POLICY=fail")))
	})

	It("runs in degraded mode with the default policy", func() {
		conf := configuration.NewConfiguration()
		conf.APINodeEnabled = false
		Expect(resolveNodeStatusSource(conf)).To(Succeed())
		Expect(conf.ReadsNoNodeStatus()).To(BeTrue())
	})
})

// Reading the configuration ConfigMap sets every field again from the
// environment, the data or the defaults: without OPERATOR_NAMESPACE, it
// dropped the namespace found from the service account
var _ = Describe("loadConfiguration", func() {
	It("keeps the namespace found from the service account", func() {
		configMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{Name: "cnpg-controller-manager-config", Namespace: "cnpg-system"},
			Data:       map[string]string{"INHERITED_LABELS": "team"},
		}
		kubeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(configMap).Build()
		conf := configuration.NewConfiguration()
		conf.OperatorNamespace = "cnpg-system"

		_, err := loadConfiguration(context.Background(), kubeClient, configMap.Name, "", conf)
		Expect(err).ToNot(HaveOccurred())

		Expect(conf.InheritedLabels).To(ConsistOf("team"))
		Expect(conf.OperatorNamespace).To(Equal("cnpg-system"))
	})
})

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

package specs

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// lookupIn builds an environment lookup over a map, standing for the
// environment of the operator
func lookupIn(env map[string]string) func(string) (string, bool) {
	return func(name string) (string, bool) {
		value, ok := env[name]
		return value, ok
	}
}

var _ = Describe("Proxy settings of the operator", func() {
	// Goal: the settings are read the way Go reads them, so that the
	// workloads get the same proxy the operator uses
	DescribeTable("are read from the environment like Go does",
		func(env map[string]string, expected []ProxySetting) {
			Expect(operatorProxySettings(lookupIn(env))).To(Equal(expected))
		},
		Entry("none when the environment has no proxy setting",
			map[string]string{"HOME": "/home", "https_proxy_backup": "x"},
			[]ProxySetting{}),
		Entry("the upper case names",
			map[string]string{
				"HTTP_PROXY":  "http://proxy:3128",
				"HTTPS_PROXY": "http://proxy:3129",
				"NO_PROXY":    "10.0.0.0/8,.svc",
			},
			[]ProxySetting{
				{Name: "HTTP_PROXY", Value: "http://proxy:3128"},
				{Name: "HTTPS_PROXY", Value: "http://proxy:3129"},
				{Name: "NO_PROXY", Value: "10.0.0.0/8,.svc"},
			}),
		Entry("the lower case names as a fallback",
			map[string]string{"http_proxy": "http://lower:3128", "no_proxy": ".cluster.local"},
			[]ProxySetting{
				{Name: "HTTP_PROXY", Value: "http://lower:3128"},
				{Name: "NO_PROXY", Value: ".cluster.local"},
			}),
		Entry("the upper case name over the lower case one",
			map[string]string{"HTTPS_PROXY": "http://upper:3129", "https_proxy": "http://lower:3129"},
			[]ProxySetting{{Name: "HTTPS_PROXY", Value: "http://upper:3129"}}),
		Entry("the lower case name when the upper case one is empty",
			map[string]string{"HTTPS_PROXY": "", "https_proxy": "http://lower:3129"},
			[]ProxySetting{{Name: "HTTPS_PROXY", Value: "http://lower:3129"}}),
		Entry("only the settings with a value",
			map[string]string{"HTTP_PROXY": "", "http_proxy": "", "NO_PROXY": "localhost"},
			[]ProxySetting{{Name: "NO_PROXY", Value: "localhost"}}),
	)

	allSettings := []ProxySetting{
		{Name: "HTTP_PROXY", Value: "http://proxy:3128"},
		{Name: "HTTPS_PROXY", Value: "http://proxy:3129"},
		{Name: "NO_PROXY", Value: "10.0.0.0/8,.svc"},
	}

	// Goal: both spellings reach the workload, and a workload that sets any
	// proxy variable keeps full control of them
	DescribeTable("become the environment of a workload",
		func(settings []ProxySetting, workloadEnv []corev1.EnvVar, expected []corev1.EnvVar) {
			Expect(proxyEnvironment(settings, workloadEnv)).To(Equal(expected))
		},
		Entry("nothing without settings",
			[]ProxySetting{}, []corev1.EnvVar{{Name: "TZ", Value: "UTC"}}, nil),
		Entry("each setting in both spellings, in a stable order",
			allSettings, nil,
			[]corev1.EnvVar{
				{Name: "HTTP_PROXY", Value: "http://proxy:3128"},
				{Name: "http_proxy", Value: "http://proxy:3128"},
				{Name: "HTTPS_PROXY", Value: "http://proxy:3129"},
				{Name: "https_proxy", Value: "http://proxy:3129"},
				{Name: "NO_PROXY", Value: "10.0.0.0/8,.svc"},
				{Name: "no_proxy", Value: "10.0.0.0/8,.svc"},
			}),
		Entry("a single setting",
			[]ProxySetting{{Name: "NO_PROXY", Value: "localhost"}}, nil,
			[]corev1.EnvVar{
				{Name: "NO_PROXY", Value: "localhost"},
				{Name: "no_proxy", Value: "localhost"},
			}),
		Entry("the same with unrelated workload variables",
			[]ProxySetting{{Name: "NO_PROXY", Value: "localhost"}},
			[]corev1.EnvVar{{Name: "TZ", Value: "UTC"}, {Name: "HTTP_PROXY_PORT", Value: "3128"}},
			[]corev1.EnvVar{
				{Name: "NO_PROXY", Value: "localhost"},
				{Name: "no_proxy", Value: "localhost"},
			}),
		Entry("nothing when the workload sets a proxy variable in upper case",
			allSettings, []corev1.EnvVar{{Name: "HTTPS_PROXY", Value: "http://other:3129"}}, nil),
		Entry("nothing when the workload sets a proxy variable in lower case",
			allSettings, []corev1.EnvVar{{Name: "no_proxy", Value: "*"}}, nil),
		Entry("nothing when the workload sets a proxy variable in mixed case",
			allSettings, []corev1.EnvVar{{Name: "Http_Proxy", Value: "http://other:3128"}}, nil),
		Entry("nothing when the workload sets a proxy variable to an empty value",
			allSettings, []corev1.EnvVar{{Name: "HTTP_PROXY", Value: ""}}, nil),
		Entry("nothing when the workload reads a proxy variable from a Secret",
			allSettings,
			[]corev1.EnvVar{{
				Name: "HTTPS_PROXY",
				ValueFrom: &corev1.EnvVarSource{
					SecretKeyRef: &corev1.SecretKeySelector{Key: "url"},
				},
			}},
			nil),
	)

	// Goal: the startup log shows which proxy is used without leaking credentials
	DescribeTable("are logged without credentials",
		func(setting ProxySetting, expected string, valid bool) {
			Expect(setting.Redacted()).To(Equal(expected))
			if valid {
				Expect(setting.Validate()).To(Succeed())
			} else {
				Expect(setting.Validate()).ToNot(Succeed())
			}
		},
		Entry("a plain URL", ProxySetting{Name: "HTTP_PROXY", Value: "http://proxy:3128"}, "http://proxy:3128", true),
		Entry("a URL with credentials",
			ProxySetting{Name: "HTTPS_PROXY", Value: "http://login:secret@proxy:3128/"}, "http://REDACTED@proxy:3128/", true),
		Entry("a URL with a user only",
			ProxySetting{Name: "HTTPS_PROXY", Value: "socks5://login@proxy:1080"}, "socks5://REDACTED@proxy:1080", true),
		Entry("a host and port without scheme, read as http like Go does",
			ProxySetting{Name: "HTTP_PROXY", Value: "proxy:3128"}, "http://proxy:3128", true),
		Entry("a value that is not a URL",
			ProxySetting{Name: "HTTP_PROXY", Value: "http://login:se%cret@proxy"}, "(not a valid URL)", false),
		Entry("NO_PROXY as is",
			ProxySetting{Name: "NO_PROXY", Value: "10.0.0.0/8,.svc,login@host"}, "10.0.0.0/8,.svc,login@host", true),
	)

	// Goal: the workloads get the settings unless PROPAGATE_PROXY_ENV is switched off
	Context("reach the workloads unless PROPAGATE_PROXY_ENV is false", func() {
		BeforeEach(func() {
			GinkgoT().Setenv("HTTPS_PROXY", "http://proxy:3129")
			GinkgoT().Setenv("https_proxy", "")
			GinkgoT().Setenv("HTTP_PROXY", "")
			GinkgoT().Setenv("http_proxy", "")
			GinkgoT().Setenv("NO_PROXY", "")
			GinkgoT().Setenv("no_proxy", "")

			previous := configuration.Current.PropagateProxyEnv
			DeferCleanup(func() { configuration.Current.PropagateProxyEnv = previous })
		})

		It("gives the settings of the operator by default", func() {
			Expect(configuration.Current.PropagateProxyEnv).To(BeTrue())
			Expect(ProxyEnvironment(nil)).To(Equal([]corev1.EnvVar{
				{Name: "HTTPS_PROXY", Value: "http://proxy:3129"},
				{Name: "https_proxy", Value: "http://proxy:3129"},
			}))
			Expect(ProxyEnvironment([]corev1.EnvVar{{Name: "https_proxy", Value: ""}})).To(BeEmpty())
		})

		It("gives nothing when switched off", func() {
			configuration.Current.PropagateProxyEnv = false
			Expect(ProxyEnvironment(nil)).To(BeEmpty())
		})
	})
})

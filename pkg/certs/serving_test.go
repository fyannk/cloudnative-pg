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

package certs

import (
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Goal: an externally provided webhook certificate is found under either file
// name convention, and anything short of a complete, readable pair stops the
// operator with a message that says what is wrong
var _ = Describe("FindServingCertificate", func() {
	var dir string

	// write creates the given files in the certificate directory
	write := func(names ...string) {
		for _, name := range names {
			Expect(os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600)).To(Succeed())
		}
	}

	BeforeEach(func() {
		dir = GinkgoT().TempDir()
	})

	DescribeTable("accepts a complete pair",
		func(files []string, expected ServingCertificateFiles) {
			write(files...)
			Expect(FindServingCertificate(dir)).To(Equal(expected))
		},
		Entry("in the OLM convention",
			[]string{"apiserver.crt", "apiserver.key"},
			ServingCertificateFiles{CertName: "apiserver.crt", KeyName: "apiserver.key"}),
		Entry("in the Kubernetes TLS Secret convention",
			[]string{"tls.crt", "tls.key"},
			ServingCertificateFiles{CertName: "tls.crt", KeyName: "tls.key"}),
		Entry("in the Kubernetes TLS Secret convention next to a CA certificate",
			[]string{"tls.crt", "tls.key", "ca.crt"},
			ServingCertificateFiles{CertName: "tls.crt", KeyName: "tls.key"}),
		Entry("preferring the OLM convention when both pairs are present",
			[]string{"apiserver.crt", "apiserver.key", "tls.crt", "tls.key"},
			ServingCertificateFiles{CertName: "apiserver.crt", KeyName: "apiserver.key"}),
		Entry("ignoring an incomplete pair of the other convention next to a complete one",
			[]string{"apiserver.crt", "apiserver.key", "tls.crt"},
			ServingCertificateFiles{CertName: "apiserver.crt", KeyName: "apiserver.key"}),
	)

	DescribeTable("refuses an incomplete pair",
		func(files []string, message string) {
			write(files...)
			_, err := FindServingCertificate(dir)
			Expect(err).To(MatchError(ContainSubstring(message)))
		},
		Entry("the OLM certificate without its key",
			[]string{"apiserver.crt"}, `webhook private key "apiserver.key"`),
		Entry("the OLM key without its certificate",
			[]string{"apiserver.key"}, `webhook certificate "apiserver.crt"`),
		Entry("the TLS certificate without its key",
			[]string{"tls.crt"}, `webhook private key "tls.key"`),
		Entry("the TLS key without its certificate",
			[]string{"tls.key", "ca.crt"}, `webhook certificate "tls.crt"`),
	)

	It("names both conventions when the directory holds no pair", func() {
		write("ca.crt", "something.pem")
		_, err := FindServingCertificate(dir)
		Expect(err).To(MatchError(ContainSubstring("apiserver.crt/apiserver.key or tls.crt/tls.key")))
		Expect(err).To(MatchError(ContainSubstring(dir)))
	})

	It("fails on a missing directory", func() {
		_, err := FindServingCertificate(filepath.Join(dir, "missing"))
		Expect(err).To(MatchError(ContainSubstring("webhook certificate directory")))
		Expect(err).To(MatchError(os.ErrNotExist))
	})

	It("fails when the path is a file", func() {
		write("tls.crt")
		_, err := FindServingCertificate(filepath.Join(dir, "tls.crt"))
		Expect(err).To(MatchError(ContainSubstring("is not a directory")))
	})

	It("fails when a file of the pair is not a regular file", func() {
		write("tls.crt")
		Expect(os.Mkdir(filepath.Join(dir, "tls.key"), 0o700)).To(Succeed())
		_, err := FindServingCertificate(dir)
		Expect(err).To(MatchError(ContainSubstring("is not a regular file")))
	})

	It("fails when a file of the pair cannot be read", func() {
		if os.Geteuid() == 0 {
			Skip("root reads every file")
		}
		write("tls.crt", "tls.key")
		Expect(os.Chmod(filepath.Join(dir, "tls.key"), 0o000)).To(Succeed())
		_, err := FindServingCertificate(dir)
		Expect(err).To(MatchError(ContainSubstring(`webhook private key "tls.key"`)))
		Expect(err).To(MatchError(os.ErrPermission))
	})
})

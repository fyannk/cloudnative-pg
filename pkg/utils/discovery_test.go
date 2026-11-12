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

package utils

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	discoveryFake "k8s.io/client-go/discovery/fake"
	fakeClient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = DescribeTable("Kubernetes minor version detection",
	func(info *version.Info, detectedMinorVersion int, shouldSucceed bool) {
		result, err := extractK8sMinorVersion(info)
		Expect(result).To(Equal(detectedMinorVersion))
		Expect(err == nil).To(Equal(shouldSucceed))
	},
	Entry("When minor version is an integer", &version.Info{Minor: "25"}, 25, true),
	Entry("When minor version indicate backported patches", &version.Info{Minor: "21+"}, 21, true),
	Entry("When minor version is wrong", &version.Info{Minor: "c3p0"}, 0, false),
)

var _ = Describe("Detect resources properly when", func() {
	var client *fakeClient.Clientset
	var fakeDiscovery *discoveryFake.FakeDiscovery

	BeforeEach(func() {
		client = fakeClient.NewClientset()
		fakeDiscovery = client.Discovery().(*discoveryFake.FakeDiscovery)
	})

	It("should not detect PodMonitor resource", func() {
		exists, err := PodMonitorExist(client.Discovery())
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeFalse())
	})

	It("should detect PodMonitor resource", func() {
		resources := []*metav1.APIResourceList{
			{
				GroupVersion: "monitoring.coreos.com/v1",
				APIResources: []metav1.APIResource{
					{
						Name: "podmonitors",
					},
				},
			},
		}
		fakeDiscovery.Resources = resources
		exists, err := PodMonitorExist(client.Discovery())
		Expect(err).ToNot(HaveOccurred())
		Expect(exists).To(BeTrue())
	})

	It("should not detect SecurityContextConstraints", func() {
		err := DetectSecurityContextConstraints(client.Discovery())
		Expect(err).ToNot(HaveOccurred())

		Expect(HaveSecurityContextConstraints()).To(BeFalse())
	})

	It("should detect SecurityContextConstraints resource", func() {
		resources := []*metav1.APIResourceList{
			{
				GroupVersion: "security.openshift.io/v1",
				APIResources: []metav1.APIResource{
					{
						Name: "securitycontextconstraints",
					},
				},
			},
		}
		fakeDiscovery.Resources = resources
		err := DetectSecurityContextConstraints(client.Discovery())
		Expect(err).ToNot(HaveOccurred())

		Expect(HaveSecurityContextConstraints()).To(BeTrue())
	})
})

// forbiddenNamespaceReader answers Forbidden to a List in one namespace and
// serves every other List
type forbiddenNamespaceReader struct {
	client.Reader
	forbidden string
}

func (r forbiddenNamespaceReader) List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error {
	options := &client.ListOptions{}
	options.ApplyOptions(opts)
	if options.Namespace == r.forbidden {
		return apierrors.NewForbidden(schema.GroupResource{Resource: "configmaps"}, "", errors.New("denied"))
	}
	return r.Reader.List(ctx, list, opts...)
}

// The probe answers the question an informer would ask: can this kind be
// listed where it will be watched. Forbidden, not installed and an unknown
// kind disable; a transient error must not.
var _ = Describe("Probing an optional API", func() {
	listError := func(err error) client.Reader {
		return fake.NewClientBuilder().WithScheme(scheme.Scheme).WithInterceptorFuncs(interceptor.Funcs{
			List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error {
				return err
			},
		}).Build()
	}

	It("reports an API that can be listed", func(ctx SpecContext) {
		reader := fake.NewClientBuilder().WithScheme(scheme.Scheme).Build()
		Expect(ProbeListable(ctx, reader, &corev1.ConfigMapList{}, nil)).To(BeTrue())
	})

	It("disables an API the operator is forbidden to list, or not installed", func(ctx SpecContext) {
		for _, err := range []error{
			apierrors.NewForbidden(schema.GroupResource{Resource: "configmaps"}, "", errors.New("denied")),
			apierrors.NewNotFound(schema.GroupResource{Resource: "configmaps"}, ""),
			&meta.NoKindMatchError{GroupKind: schema.GroupKind{Kind: "PodMonitor"}},
		} {
			available, probeErr := ProbeListable(ctx, listError(err), &corev1.ConfigMapList{}, nil)
			Expect(probeErr).ToNot(HaveOccurred())
			Expect(available).To(BeFalse())
		}
	})

	It("returns any other error instead of disabling the API", func(ctx SpecContext) {
		_, err := ProbeListable(ctx, listError(errors.New("timeout")), &corev1.ConfigMapList{}, nil)
		Expect(err).To(MatchError("timeout"))
	})

	It("probes every watched namespace and disables on the first forbidden one", func(ctx SpecContext) {
		reader := forbiddenNamespaceReader{
			Reader:    fake.NewClientBuilder().WithScheme(scheme.Scheme).Build(),
			forbidden: "locked",
		}
		Expect(ProbeListable(ctx, reader, &corev1.ConfigMapList{}, []string{"open"})).To(BeTrue())
		Expect(ProbeListable(ctx, reader, &corev1.ConfigMapList{}, []string{"open", "locked"})).To(BeFalse())
	})
})

var _ = Describe("AvailableArchitecture", func() {
	var (
		mockHashCalculator func(_ string) (hash string, err error)
		arch               *AvailableArchitecture
	)

	BeforeEach(func() {
		mockHashCalculator = func(_ string) (hash string, err error) {
			return "mockedHash", nil
		}
		arch = newAvailableArchitecture("amd64", filepath.Join("bin", "manager_amd64"))
		arch.hashCalculator = mockHashCalculator
	})

	Describe("GetHash", func() {
		Context("when hash is not calculated yet", func() {
			It("should calculate the hash", func() {
				hash := arch.GetHash()
				Expect(hash).To(Equal("mockedHash"))
			})
		})

		Context("when hash is already calculated", func() {
			BeforeEach(func() {
				arch.hash = "precalculatedHash"
			})

			It("should return the precalculated hash", func() {
				hash := arch.GetHash()
				Expect(hash).To(Equal("precalculatedHash"))
			})
		})
	})

	Describe("calculateHash", func() {
		Context("when hash is not calculated yet", func() {
			It("should calculate the hash", func() {
				arch.calculateHash()
				Expect(arch.hash).To(Equal("mockedHash"))
			})
		})

		Context("when hash is already calculated", func() {
			BeforeEach(func() {
				mockHashCalculator = func(_ string) (hash string, err error) {
					return "should-not-return-this", nil
				}
				arch.hash = "precalculatedHash"
			})

			It("should not recalculate the hash on subsequent calls", func() {
				arch.calculateHash()
				hash1 := arch.hash

				arch.hashCalculator = mockHashCalculator

				arch.calculateHash()
				hash2 := arch.hash

				Expect(hash1).To(Equal(hash2))
			})
		})

		Context("when hash calculation returns an error", func() {
			BeforeEach(func() {
				mockHashCalculator = func(_ string) (hash string, err error) {
					return "", fmt.Errorf("fake error")
				}
				arch.hashCalculator = mockHashCalculator
			})

			It("should panic with an error", func() {
				Expect(func() {
					arch.calculateHash()
				}).To(Panic())
			})
		})
	})

	Describe("detectAvailableArchitectures", func() {
		var tempDir string
		var err error

		BeforeEach(func() {
			tempDir, err = os.MkdirTemp("", "test")
			Expect(err).NotTo(HaveOccurred())
		})

		AfterEach(func() {
			Expect(os.RemoveAll(tempDir)).To(Succeed())
			availableArchitectures = nil
		})

		It("shouldn't find available architectures", func() {
			// Create some sample files
			Expect(os.WriteFile(filepath.Join(tempDir, "test1"), []byte("amd64"), 0o600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tempDir, "test2"), []byte("arm64"), 0o600)).To(Succeed())

			err = detectAvailableArchitectures(filepath.Join(tempDir, "manager_*"))
			Expect(err).ToNot(HaveOccurred())
			Expect(availableArchitectures).To(BeNil())

			architectures := GetAvailableArchitectures()
			Expect(architectures).To(BeEmpty())
		})

		It("should find available architectures", func() {
			expectedArchitectures := []string{"amd64", "arm64"}

			// Create some sample files
			Expect(os.WriteFile(filepath.Join(tempDir, "manager_amd64"), []byte("amd64"), 0o600)).To(Succeed())
			Expect(os.WriteFile(filepath.Join(tempDir, "manager_arm64"), []byte("arm64"), 0o600)).To(Succeed())

			err = detectAvailableArchitectures(filepath.Join(tempDir, "manager_*"))
			Expect(err).ToNot(HaveOccurred())
			Expect(availableArchitectures).To(HaveLen(2))

			Eventually(func(g Gomega) {
				architectures := GetAvailableArchitectures()
				Expect(architectures).NotTo(BeEmpty())
				for _, a := range architectures {
					g.Expect(slices.Contains(expectedArchitectures, a.GoArch)).To(BeTrue())
					g.Expect(a.GetHash()).ToNot(BeEmpty())
				}
			}).Should(Succeed())
		})
	})

	Describe("GetAvailableArchitecture", func() {
		It("should fail when retrieving an architecture that doesn't exist", func() {
			availableArch, err := GetAvailableArchitecture("arm64")
			Expect(err).To(HaveOccurred())
			Expect(availableArch).To(BeNil())
		})

		It("should retrieve an existing available architecture", func() {
			tempDir := GinkgoT().TempDir()
			DeferCleanup(func() {
				availableArchitectures = nil
			})

			// Create a sample file
			Expect(os.WriteFile(filepath.Join(tempDir, "manager_amd64"), []byte("amd64"), 0o600)).To(Succeed())
			err := detectAvailableArchitectures(filepath.Join(tempDir, "manager_*"))
			Expect(err).ToNot(HaveOccurred())
			Expect(availableArchitectures).To(HaveLen(1))

			Eventually(func(g Gomega) {
				availableArch, err := GetAvailableArchitecture("amd64")
				g.Expect(err).ToNot(HaveOccurred())
				g.Expect(availableArch.GoArch).To(BeEquivalentTo("amd64"))
				g.Expect(availableArch.GetHash()).ToNot(BeEmpty())
			}).Should(Succeed())
		})
	})
})

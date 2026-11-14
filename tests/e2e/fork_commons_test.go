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

package e2e

import (
	"maps"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/cloudnative-pg/cloudnative-pg/tests/utils/operator"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Helpers shared by the specs of the features this fork adds to
// CloudNativePG, the ones labeled tests.LabelFork.

const (
	// forkFixturesDir is where the fixtures of the fork specs live
	forkFixturesDir = fixturesDir + "/fork"

	// operatorConfigMapName is the name of the ConfigMap the operator reads
	// its configuration from when it starts
	operatorConfigMapName = "cnpg-controller-manager-config"

	// operatorReloadTimeout is the time, in seconds, given to the operator
	// to be ready again after a restart
	operatorReloadTimeout = 180
)

// saveOperatorConfiguration records the operator ConfigMap, or its absence,
// and returns the function putting it back and restarting the operator.
// The specs changing the operator configuration register it with
// DeferCleanup before their first change.
func saveOperatorConfiguration() func() {
	GinkgoHelper()

	operatorNamespace, err := operator.NamespaceName(env.Ctx, env.Client)
	Expect(err).ToNot(HaveOccurred())

	key := ctrlclient.ObjectKey{Namespace: operatorNamespace, Name: operatorConfigMapName}
	initial := &corev1.ConfigMap{}
	err = env.Client.Get(env.Ctx, key, initial)
	existed := !apierrors.IsNotFound(err)
	if existed {
		Expect(err).ToNot(HaveOccurred())
	}

	return func() {
		GinkgoHelper()

		err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
			current := &corev1.ConfigMap{}
			err := env.Client.Get(env.Ctx, key, current)
			switch {
			case apierrors.IsNotFound(err) && !existed:
				return nil
			case apierrors.IsNotFound(err):
				return env.Client.Create(env.Ctx, &corev1.ConfigMap{
					ObjectMeta: metav1.ObjectMeta{
						Namespace:   key.Namespace,
						Name:        key.Name,
						Labels:      initial.Labels,
						Annotations: initial.Annotations,
					},
					Data: initial.Data,
				})
			case err != nil:
				return err
			case !existed:
				return env.Client.Delete(env.Ctx, current)
			}
			current.Data = initial.Data
			return env.Client.Update(env.Ctx, current)
		})
		Expect(err).ToNot(HaveOccurred())

		Expect(operator.ReloadDeployment(env.Ctx, env.Client, operatorReloadTimeout)).To(Succeed())
	}
}

// setOperatorConfiguration sets the given settings in the operator
// ConfigMap, creating it when missing and leaving its other settings as
// they are, then restarts the operator, which reads it when it starts.
func setOperatorConfiguration(settings map[string]string) {
	GinkgoHelper()

	operatorNamespace, err := operator.NamespaceName(env.Ctx, env.Client)
	Expect(err).ToNot(HaveOccurred())

	key := ctrlclient.ObjectKey{Namespace: operatorNamespace, Name: operatorConfigMapName}
	err = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		configMap := &corev1.ConfigMap{}
		err := env.Client.Get(env.Ctx, key, configMap)
		if apierrors.IsNotFound(err) {
			return env.Client.Create(env.Ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{Namespace: key.Namespace, Name: key.Name},
				Data:       settings,
			})
		}
		if err != nil {
			return err
		}
		if configMap.Data == nil {
			configMap.Data = map[string]string{}
		}
		maps.Copy(configMap.Data, settings)
		return env.Client.Update(env.Ctx, configMap)
	})
	Expect(err).ToNot(HaveOccurred())

	Expect(operator.ReloadDeployment(env.Ctx, env.Client, operatorReloadTimeout)).To(Succeed())
}

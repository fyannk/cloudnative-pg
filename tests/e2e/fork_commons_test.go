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

	appsv1 "k8s.io/api/apps/v1"
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

// setOperatorDeploymentEnv replaces the environment variables of the
// operator container having the names of the given ones, adds the others,
// and waits for the operator to be rolled out. It returns the environment
// the container had before.
func setOperatorDeploymentEnv(variables []corev1.EnvVar) []corev1.EnvVar {
	GinkgoHelper()

	var previous []corev1.EnvVar
	updateOperatorDeploymentEnv(func(current []corev1.EnvVar) []corev1.EnvVar {
		previous = append([]corev1.EnvVar(nil), current...)

		result := make([]corev1.EnvVar, 0, len(current)+len(variables))
		for _, envVar := range current {
			replaced := false
			for _, variable := range variables {
				replaced = replaced || variable.Name == envVar.Name
			}
			if !replaced {
				result = append(result, envVar)
			}
		}
		return append(result, variables...)
	})
	return previous
}

// restoreOperatorDeploymentEnv puts back the environment returned by
// setOperatorDeploymentEnv, and waits for the operator to be rolled out
func restoreOperatorDeploymentEnv(previous []corev1.EnvVar) {
	GinkgoHelper()
	updateOperatorDeploymentEnv(func([]corev1.EnvVar) []corev1.EnvVar { return previous })
}

func updateOperatorDeploymentEnv(mutate func(current []corev1.EnvVar) []corev1.EnvVar) {
	GinkgoHelper()

	var key ctrlclient.ObjectKey
	err := retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		deployment, err := operator.GetDeployment(env.Ctx, env.Client)
		if err != nil {
			return err
		}
		key = ctrlclient.ObjectKeyFromObject(&deployment)
		container := &deployment.Spec.Template.Spec.Containers[0]
		container.Env = mutate(container.Env)
		return env.Client.Update(env.Ctx, &deployment)
	})
	Expect(err).ToNot(HaveOccurred())

	// the rollout is over once the only Pod left is one of the new template
	Eventually(func(g Gomega) {
		deployment := &appsv1.Deployment{}
		g.Expect(env.Client.Get(env.Ctx, key, deployment)).To(Succeed())
		g.Expect(deployment.Status.ObservedGeneration).To(Equal(deployment.Generation))
		g.Expect(deployment.Status.UpdatedReplicas).To(Equal(*deployment.Spec.Replicas))
		g.Expect(deployment.Status.Replicas).To(Equal(*deployment.Spec.Replicas))
		g.Expect(deployment.Status.AvailableReplicas).To(Equal(*deployment.Spec.Replicas))

		_, err := operator.GetPod(env.Ctx, env.Client)
		g.Expect(err).ToNot(HaveOccurred())
	}, operatorReloadTimeout).Should(Succeed())

	Expect(operator.WaitForReady(env.Ctx, env.Client, operatorReloadTimeout, true)).To(Succeed())
}

// postgresContainerEnv returns the environment variables set in the
// specification of the PostgreSQL container of an instance Pod, by name
func postgresContainerEnv(pod corev1.Pod) map[string]string {
	result := map[string]string{}
	for _, container := range pod.Spec.Containers {
		if container.Name != "postgres" {
			continue
		}
		for _, envVar := range container.Env {
			result[envVar.Name] = envVar.Value
		}
	}
	return result
}

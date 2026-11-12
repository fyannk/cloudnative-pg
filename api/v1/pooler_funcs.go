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

package v1

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/url"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/metadatarules"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/monitoringdiscovery"
)

// IsPaused returns whether all database should be paused or not.
func (in PgBouncerSpec) IsPaused() bool {
	return in.Paused != nil && *in.Paused
}

// GetAuthQuerySecretName returns the specified AuthQuerySecret name for PgBouncer
// if provided or the default name otherwise.
func (in *Pooler) GetAuthQuerySecretName() string {
	if in.Spec.PgBouncer != nil && in.Spec.PgBouncer.AuthQuerySecret != nil {
		return in.Spec.PgBouncer.AuthQuerySecret.Name
	}

	return in.Spec.Cluster.Name + DefaultPgBouncerPoolerSecretSuffix
}

// GetServerTLSSecretName returns the specified server TLS secret name
// for PgBouncer if provided or the default name otherwise.
func (in *Pooler) GetServerTLSSecretName() string {
	if in.Spec.PgBouncer != nil && in.Spec.PgBouncer.ServerTLSSecret != nil {
		return in.Spec.PgBouncer.ServerTLSSecret.Name
	}

	return ""
}

// GetServerCASecretNameOrDefault returns the specified server CA secret name
// for PgBouncer if provided or the default name otherwise.
func (in *Pooler) GetServerCASecretNameOrDefault(cluster *Cluster) string {
	if in.Spec.PgBouncer != nil && in.Spec.PgBouncer.ServerCASecret != nil {
		return in.Spec.PgBouncer.ServerCASecret.Name
	}

	return cluster.GetServerCASecretName()
}

// GetClientCASecretNameOrDefault returns the specified client CA secret name
// for PgBouncer if provided or the default name otherwise.
func (in *Pooler) GetClientCASecretNameOrDefault(cluster *Cluster) string {
	if in.Spec.PgBouncer != nil && in.Spec.PgBouncer.ClientCASecret != nil {
		return in.Spec.PgBouncer.ClientCASecret.Name
	}

	return cluster.GetClientCASecretName()
}

// GetClientTLSSecretNameOrDefault returns the specified client TLS secret name
// for PgBouncer if provided or the default name otherwise.
func (in *Pooler) GetClientTLSSecretNameOrDefault(cluster *Cluster) string {
	if in.Spec.PgBouncer != nil && in.Spec.PgBouncer.ClientTLSSecret != nil {
		return in.Spec.PgBouncer.ClientTLSSecret.Name
	}

	return cluster.GetServerTLSSecretName()
}

// GetAuthQuery returns the specified AuthQuery name for PgBouncer
// if provided or the default name otherwise.
func (in *Pooler) GetAuthQuery() string {
	if in.Spec.PgBouncer.AuthQuery != "" {
		return in.Spec.PgBouncer.AuthQuery
	}

	return DefaultPgBouncerPoolerAuthQuery
}

// IsAutomatedIntegration returns whether the Pooler integration with the
// Cluster is automated or not.
func (in *Pooler) IsAutomatedIntegration() bool {
	if in.Spec.PgBouncer == nil {
		return true
	}
	// If the user specified an AuthQuerySecret or an AuthQuery, the integration
	// is not going to be handled by the operator.
	if in.Spec.PgBouncer.AuthQuery != "" ||
		(in.Spec.PgBouncer.AuthQuerySecret != nil && in.Spec.PgBouncer.AuthQuerySecret.Name != "") ||
		(in.Spec.PgBouncer.ServerTLSSecret != nil && in.Spec.PgBouncer.ServerTLSSecret.Name != "") {
		return false
	}
	return true
}

// GetResourcesRequirements returns the resource requirements for the Pooler
func (in *Pooler) GetResourcesRequirements() corev1.ResourceRequirements {
	if in.Spec.Template == nil {
		return corev1.ResourceRequirements{}
	}

	if in.Spec.Template.Spec.Resources == nil {
		return corev1.ResourceRequirements{}
	}

	return *in.Spec.Template.Spec.Resources
}

// GetServiceAccountName returns the name of the ServiceAccount for this pooler.
func (in *Pooler) GetServiceAccountName() string {
	if in.Spec.ServiceAccountName != "" {
		return in.Spec.ServiceAccountName
	}
	return in.Name
}

// IsMetricsTLSEnabled checks if the metrics endpoint should use TLS
func (in *Pooler) IsMetricsTLSEnabled() bool {
	if in.Spec.Monitoring != nil && in.Spec.Monitoring.TLSConfig != nil {
		return in.Spec.Monitoring.TLSConfig.Enabled
	}

	return false
}

// GetMonitoringDiscoveryAnnotations gets the annotations telling monitoring
// agents how to collect the metrics and the logs of the Pooler Pods, for the
// families of MONITORING_DISCOVERY_ANNOTATIONS. Nil when there is none
func (in *Pooler) GetMonitoringDiscoveryAnnotations() map[string]string {
	return monitoringdiscovery.Annotations(configuration.Current.MonitoringDiscoveryFamilies(),
		monitoringdiscovery.Endpoint{
			MetricsPort:   url.PgBouncerMetricsPort,
			MetricsTLS:    in.IsMetricsTLSEnabled(),
			JobName:       "cloudnative-pg-pooler",
			LogsContainer: "pgbouncer",
		})
}

// SetAdmissionError sets the admission error status on the Pooler resource
func (in *Pooler) SetAdmissionError(msg string) {
	in.Status.Error = msg
}

// GetAdmissionError returns the admission error recorded on the Pooler status
func (in *Pooler) GetAdmissionError() string {
	return in.Status.Error
}

// PoolerMetadataTargets are the kinds of objects the metadataPropagation
// rules of a Pooler can target: the objects it creates
var PoolerMetadataTargets = metadatarules.PoolerResource.Targets()

// GetMetadataRules compiles the metadataPropagation rules of the Pooler.
// The webhook reports their problems; the valid part of the rules applies
func (in *Pooler) GetMetadataRules() metadatarules.Set {
	set, _ := metadatarules.Compile(ToMetadataRules(in.Spec.MetadataPropagation), PoolerMetadataTargets)
	return set
}

// ApplyMetadataTo stacks, on the metadata of an object of the Pooler of a
// kind, the default metadata of the operator below the labels and
// annotations the object already has, and the rules of the Pooler and the
// mandatory metadata of the operator above them
func (in *Pooler) ApplyMetadataTo(meta *metav1.ObjectMeta, target metadatarules.Target) {
	applyMetadataRules(meta, metadatarules.PoolerResource, in.GetMetadataRules(), target)
}

// MetadataReaches tells whether a rule of the Pooler or of the operator puts
// a label or an annotation on the objects of a kind
func (in *Pooler) MetadataReaches(target metadatarules.Target) bool {
	return configuration.Current.MetadataLayers(metadatarules.PoolerResource, in.GetMetadataRules()).Reaches(target)
}

// GetPoolerLabelsFor gets the labels the Pooler sets on a kind of object:
// those of its templates, with its rules on top
func (in *Pooler) GetPoolerLabelsFor(target metadatarules.Target) map[string]string {
	template := in.templateMetadataFor(target)
	return metadatarules.Layers{Own: in.GetMetadataRules()}.Labels(target, template.Labels)
}

// GetPoolerAnnotationsFor gets the annotations the Pooler sets on a kind of
// object, as GetPoolerLabelsFor does for the labels
func (in *Pooler) GetPoolerAnnotationsFor(target metadatarules.Target) map[string]string {
	template := in.templateMetadataFor(target)
	return metadatarules.Layers{Own: in.GetMetadataRules()}.Annotations(target, template.Annotations)
}

// templateMetadataFor returns the metadata of the template of a kind of
// object: the pod template for the Pods, the service template for the
// Service, none for the others
func (in *Pooler) templateMetadataFor(target metadatarules.Target) Metadata {
	switch {
	case target == metadatarules.PoolerPods && in.Spec.Template != nil:
		return in.Spec.Template.ObjectMeta
	case target == metadatarules.Services && in.Spec.ServiceTemplate != nil:
		return in.Spec.ServiceTemplate.ObjectMeta
	}
	return Metadata{}
}

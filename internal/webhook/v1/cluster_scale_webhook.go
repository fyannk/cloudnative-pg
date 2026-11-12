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
	"context"
	"errors"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
)

// clusterScaleValidationPath is the path of the webhook validating the
// writes of the scale subresource of a Cluster
const clusterScaleValidationPath = "/validate-postgresql-cnpg-io-v1-cluster-scale"

// SetupClusterScaleWebhookWithManager registers the webhook validating the
// writes of the scale subresource of a Cluster in the manager.
func SetupClusterScaleWebhookWithManager(mgr ctrl.Manager) {
	mgr.GetWebhookServer().Register(clusterScaleValidationPath, &webhook.Admission{
		Handler: &ClusterScaleValidator{
			reader:    mgr.GetAPIReader(),
			decoder:   admission.NewDecoder(mgr.GetScheme()),
			validator: newBypassableValidator[*apiv1.Cluster](&ClusterCustomValidator{}),
		},
	})
}

// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:webhookVersions={v1},admissionReviewVersions={v1},verbs=update,path=/validate-postgresql-cnpg-io-v1-cluster-scale,mutating=false,failurePolicy=fail,groups=postgresql.cnpg.io,resources=clusters/scale,versions=v1,name=vclusterscale.cnpg.io,sideEffects=None

// ClusterScaleValidator validates the writes of the scale subresource of a
// Cluster, used by `kubectl scale` and the autoscalers. Such a request
// carries an autoscaling/v1 Scale, not a Cluster, so the webhook of the
// Cluster never sees it: this one applies the requested number of instances
// to the stored Cluster and validates the result as an update of the Cluster.
type ClusterScaleValidator struct {
	reader    client.Reader
	decoder   admission.Decoder
	validator admission.Validator[*apiv1.Cluster]
}

// Handle implements admission.Handler
func (v *ClusterScaleValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
	if req.Operation != admissionv1.Update {
		return admission.Allowed("")
	}

	var scale autoscalingv1.Scale
	if err := v.decoder.Decode(req, &scale); err != nil {
		return admission.Errored(http.StatusBadRequest, err)
	}

	var cluster apiv1.Cluster
	if err := v.reader.Get(ctx, types.NamespacedName{Namespace: req.Namespace, Name: req.Name}, &cluster); err != nil {
		return admission.Errored(http.StatusInternalServerError, err)
	}

	if cluster.Spec.Instances == int(scale.Spec.Replicas) {
		return admission.Allowed("")
	}

	clusterLog.Debug("Validation for Cluster upon scale", "name", cluster.GetName(), "namespace",
		cluster.GetNamespace(), "instances", cluster.Spec.Instances, "replicas", scale.Spec.Replicas)

	scaled := cluster.DeepCopy()
	scaled.Spec.Instances = int(scale.Spec.Replicas)

	warnings, err := v.validator.ValidateUpdate(ctx, &cluster, scaled)
	if err == nil {
		return admission.Allowed("").WithWarnings(warnings...)
	}

	var apiStatus apierrors.APIStatus
	if errors.As(err, &apiStatus) {
		status := apiStatus.Status()
		return admission.Response{
			AdmissionResponse: admissionv1.AdmissionResponse{
				Allowed: false,
				Result:  &status,
			},
		}.WithWarnings(warnings...)
	}

	return admission.Denied(err.Error()).WithWarnings(warnings...)
}

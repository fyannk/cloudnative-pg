/*
Copyright The CloudNativePG Contributors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1

import (
	"fmt"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/machinery/pkg/log"
	"github.com/cloudnative-pg/machinery/pkg/stringset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var (
	// poolerLog is for logging in this package.
	poolerLog = log.WithName("pooler-resource").WithValues("version", "v1")

	// AllowedPgbouncerGenericConfigurationParameters is the list of allowed parameters for PgBouncer
	AllowedPgbouncerGenericConfigurationParameters = stringset.From([]string{
		"application_name_add_host",
		"autodb_idle_timeout",
		"client_idle_timeout",
		"client_login_timeout",
		"default_pool_size",
		"disable_pqexec",
		"idle_transaction_timeout",
		"ignore_startup_parameters",
		"log_connections",
		"log_disconnections",
		"log_pooler_errors",
		"log_stats",
		"max_client_conn",
		"max_db_connections",
		"max_prepared_statements",
		"max_user_connections",
		"min_pool_size",
		"query_timeout",
		"query_wait_timeout",
		"reserve_pool_size",
		"reserve_pool_timeout",
		"server_check_delay",
		"server_check_query",
		"server_connect_timeout",
		"server_fast_close",
		"server_idle_timeout",
		"server_lifetime",
		"server_login_retry",
		"server_reset_query",
		"server_reset_query_always",
		"server_round_robin",
		"stats_period",
		"tcp_keepalive",
		"tcp_keepcnt",
		"tcp_keepidle",
		"tcp_keepintvl",
		"tcp_user_timeout",
		"verbose",
	})
)

// SetupWebhookWithManager setup the webhook inside the controller manager
func (r *Pooler) SetupWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(r).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// +kubebuilder:webhook:webhookVersions={v1},admissionReviewVersions={v1},verbs=create;update,path=/validate-postgresql-cnpg-io-v1-pooler,mutating=false,failurePolicy=fail,groups=postgresql.cnpg.io,resources=poolers,versions=v1,name=vpooler.cnpg.io,sideEffects=None

var _ webhook.Validator = &Pooler{}

// ValidateCreate implements webhook.Validator so a webhook will be registered for the type
func (r *Pooler) ValidateCreate() (warns admission.Warnings, err error) {
	poolerLog.Info("validate create", "name", r.Name, "namespace", r.Namespace)

	if !r.IsAutomatedIntegration() {
		poolerLog.Info("Pooler not automatically configured, manual configuration required",
			"name", r.Name, "namespace", r.Namespace, "cluster", r.Spec.Cluster.Name)
		warns = append(warns, fmt.Sprintf("The operator won't handle the Pooler %q integration with the Cluster %q (%q). "+
			"Manually configure it as described in the docs.", r.Name, r.Spec.Cluster.Name, r.Namespace))
	}

	allErrs := r.Validate()

	if len(allErrs) == 0 {
		return warns, nil
	}

	return nil, apierrors.NewInvalid(
		schema.GroupKind{Group: "postgresql.cnpg.io", Kind: "Pooler"},
		r.Name, allErrs)
}

// ValidateUpdate implements webhook.Validator so a webhook will be registered for the type
func (r *Pooler) ValidateUpdate(old runtime.Object) (warns admission.Warnings, err error) {
	poolerLog.Info("validate update", "name", r.Name, "namespace", r.Namespace)

	oldPooler := old.(*Pooler)
	// applying defaults before validating updates to set any new default
	oldPooler.SetDefaults()

	if oldPooler.IsAutomatedIntegration() && !r.IsAutomatedIntegration() {
		poolerLog.Info("Pooler not automatically configured, manual configuration required",
			"name", r.Name, "namespace", r.Namespace, "cluster", r.Spec.Cluster.Name)
		warns = append(warns, fmt.Sprintf("The operator won't handle the Pooler %q integration with the Cluster %q (%q). "+
			"Manually configure it as described in the docs.", r.Name, r.Spec.Cluster.Name, r.Namespace))
	}

	allErrs := r.Validate()
	if len(allErrs) == 0 {
		return nil, nil
	}

	return warns, apierrors.NewInvalid(
		schema.GroupKind{Group: "postgresql.cnpg.io", Kind: "Pooler"},
		r.Name, allErrs)
}

// ValidateDelete implements webhook.Validator so a webhook will be registered for the type
func (r *Pooler) ValidateDelete() (admission.Warnings, error) {
	poolerLog.Info("validate delete", "name", r.Name, "namespace", r.Namespace)
	return nil, nil
}

func (r *Pooler) validatePgBouncer() field.ErrorList {
	var result field.ErrorList
	switch {
	case r.Spec.PgBouncer == nil:
		result = append(result,
			field.Invalid(
				field.NewPath("spec", "pgbouncer"),
				"", "required pgbouncer configuration"))
	case r.Spec.PgBouncer.AuthQuerySecret != nil && r.Spec.PgBouncer.AuthQuerySecret.Name != "" &&
		r.Spec.PgBouncer.AuthQuery == "":
		result = append(result,
			field.Invalid(
				field.NewPath("spec", "pgbouncer", "authQuery"),
				"", "must specify an auth query when providing an auth query secret"))
	case (r.Spec.PgBouncer.AuthQuerySecret == nil || r.Spec.PgBouncer.AuthQuerySecret.Name == "") &&
		r.Spec.PgBouncer.AuthQuery != "":
		result = append(result,
			field.Invalid(
				field.NewPath("spec", "pgbouncer", "authQuerySecret", "name"),
				"", "must specify an existing auth query secret when providing an auth query secret"))
	}

	if r.Spec.PgBouncer != nil && len(r.Spec.PgBouncer.Parameters) > 0 {
		result = append(result, r.validatePgbouncerGenericParameters()...)
	}

	return result
}

func (r *Pooler) validateCluster() field.ErrorList {
	var result field.ErrorList
	if r.Spec.Cluster.Name == "" {
		result = append(result,
			field.Invalid(
				field.NewPath("spec", "cluster", "name"),
				"", "must specify a cluster name"))
	}
	if r.Spec.Cluster.Name == r.Name {
		result = append(result,
			field.Invalid(
				field.NewPath("metadata", "name"),
				r.Name, "the pooler resource cannot have the same name of a cluster"))
	}
	return result
}

// Validate validates the configuration of a Pooler, returning
// a list of errors
func (r *Pooler) Validate() (allErrs field.ErrorList) {
	r.SetDefaults()
	allErrs = append(allErrs, r.validatePgBouncer()...)
	allErrs = append(allErrs, r.validateCluster()...)
	return allErrs
}

// validatePgbouncerGenericParameters validates pgbouncer parameters
func (r *Pooler) validatePgbouncerGenericParameters() field.ErrorList {
	var result field.ErrorList

	for param := range r.Spec.PgBouncer.Parameters {
		if !AllowedPgbouncerGenericConfigurationParameters.Has(param) {
			result = append(result,
				field.Invalid(
					field.NewPath("spec", "cluster", "parameters"),
					param, "Invalid or reserved parameter"))
		}
	}
	return result
}

var _ webhook.Defaulter = &Pooler{}

// Default implements webhook.Defaulter so a webhook will be registered for the type
func (r *Pooler) Default() {
	poolerLog.Info("default", "name", r.Name, "namespace", r.Namespace)

	r.setDefaults()
}

// SetDefaults apply the defaults to undefined values in a Pooler
func (r *Pooler) SetDefaults() {
	r.setDefaults()
}

func (r *Pooler) setDefaults() {
	for _, configMandatoryLabel := range configuration.Current.GetMandatoryLabels() {
		_, ok := r.ObjectMeta.Labels[configMandatoryLabel.Name]
		if !ok {
			if r.ObjectMeta.Labels == nil {
				r.ObjectMeta.Labels = make(map[string]string)
			}
			r.ObjectMeta.Labels[configMandatoryLabel.Name] = configMandatoryLabel.Value
		}
	}

	for _, configMandatoryAnnotation := range configuration.Current.GetMandatoryAnnotations() {
		_, ok := r.ObjectMeta.Annotations[configMandatoryAnnotation.Name]
		if !ok {
			if r.ObjectMeta.Annotations == nil {
				r.ObjectMeta.Annotations = make(map[string]string)
			}
			r.ObjectMeta.Annotations[configMandatoryAnnotation.Name] = configMandatoryAnnotation.Value
		}
	}
}

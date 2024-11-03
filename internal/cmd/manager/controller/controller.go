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

// Package controller implement the command used to start the operator
package controller

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/cloudnative-pg/machinery/pkg/log"
	storagesnapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrs "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	// +kubebuilder:scaffold:imports
	apiv1 "github.com/cloudnative-pg/cloudnative-pg/api/v1"
	"github.com/cloudnative-pg/cloudnative-pg/internal/cnpi/plugin/repository"
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
	"github.com/cloudnative-pg/cloudnative-pg/internal/controller"
	schemeBuilder "github.com/cloudnative-pg/cloudnative-pg/internal/scheme"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/certs"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/management/postgres/webserver"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/multicache"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/utils"
	"github.com/cloudnative-pg/cloudnative-pg/pkg/versions"
)

var (
	scheme   = schemeBuilder.BuildWithAllKnownScheme()
	setupLog = log.WithName("setup")
)

const (
	// The name of the directory containing the TLS certificates
	defaultWebhookCertDir = "/run/secrets/cnpg.io/webhook"

	// LeaderElectionID The operator Leader Election ID
	LeaderElectionID = "db9c8771.cnpg.io"
)

// leaderElectionConfiguration contains the leader parameters that will be passed to controllerruntime.Options.
type leaderElectionConfiguration struct {
	enable        bool
	leaseDuration time.Duration
	renewDeadline time.Duration
}

// RunController is the main procedure of the operator, and is used as the
// controller-manager of the operator and as the controller of a certain
// PostgreSQL instance.
//
// This code really belongs to app/controller_manager.go but we can't put
// it here to respect the project layout created by kubebuilder.
func RunController(
	metricsAddr,
	configMapName,
	secretName string,
	leaderConfig leaderElectionConfiguration,
	pprofDebug bool,
	port int,
	conf *configuration.Data,
) error {
	ctx := context.Background()

	setupLog.Info("Starting CloudNativePG Operator",
		"version", versions.Version,
		"build", versions.Info)

	if pprofDebug {
		startPprofDebugServer(ctx)
	}

	managerOptions := ctrl.Options{
		Scheme: scheme,
		Metrics: server.Options{
			BindAddress: metricsAddr,
		},
		LeaderElection:   leaderConfig.enable,
		LeaseDuration:    &leaderConfig.leaseDuration,
		RenewDeadline:    &leaderConfig.renewDeadline,
		LeaderElectionID: LeaderElectionID,
		WebhookServer: webhook.NewServer(webhook.Options{
			Port:    port,
			CertDir: defaultWebhookCertDir,
		}),
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		LeaderElectionReleaseOnCancel: true,
	}

	// Load Configuration a first time to have all parameters merged (from configmap, secret, env, command line, ...)
	tmpKubeClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to create Kubernetes client")
		return err
	}
	err = loadConfiguration(ctx, tmpKubeClient, configMapName, secretName, conf)
	if err != nil {
		return err
	}

	switch {
	case conf.WatchNamespace != "":
		namespaces := conf.WatchedNamespaces()
		managerOptions.NewCache = multicache.DelegatingMultiNamespacedCacheBuilder(
			namespaces,
			conf.OperatorNamespace)
		setupLog.Info("Listening for changes", "watchNamespaces", namespaces)
	case configuration.Current.ClusterWideCacheFilter:
		setupLog.Info("Operator is Cluster Wide with filtered cache")
		cacheLabel := configuration.Current.GetCacheKey()
		// When listening in cluster-wide, we MUST filter cache for ConfigMaps and Secrets, are those are watched
		// Otherwire, we'll put in cache ALL ConfigMaps and ALL Secrets of the cluster...
		// We'll still query all of them though........
		curCache := map[client.Object]cache.ByObject{
			&corev1.ConfigMap{}: {
				Label: labels.SelectorFromSet(labels.Set{
					cacheLabel.Name: cacheLabel.Value,
				}),
			},
			&appsv1.Deployment{}: {
				Label: labels.SelectorFromSet(labels.Set{
					cacheLabel.Name: cacheLabel.Value,
				}),
			},
			&batchv1.Job{}: {
				Label: labels.SelectorFromSet(labels.Set{
					cacheLabel.Name: cacheLabel.Value,
				}),
			},
			&corev1.PersistentVolumeClaim{}: {
				Label: labels.SelectorFromSet(labels.Set{
					cacheLabel.Name: cacheLabel.Value,
				}),
			},
			&corev1.Pod{}: {
				Label: labels.SelectorFromSet(labels.Set{
					cacheLabel.Name: cacheLabel.Value,
				}),
			},
			&rbacv1.Role{}: {
				Label: labels.SelectorFromSet(labels.Set{
					cacheLabel.Name: cacheLabel.Value,
				}),
			},
			&rbacv1.RoleBinding{}: {
				Label: labels.SelectorFromSet(labels.Set{
					cacheLabel.Name: cacheLabel.Value,
				}),
			},
			&corev1.Secret{}: {
				Label: labels.SelectorFromSet(labels.Set{
					cacheLabel.Name: cacheLabel.Value,
				}),
			},
			&corev1.Service{}: {
				Label: labels.SelectorFromSet(labels.Set{
					cacheLabel.Name: cacheLabel.Value,
				}),
			},
			&corev1.ServiceAccount{}: {
				Label: labels.SelectorFromSet(labels.Set{
					cacheLabel.Name: cacheLabel.Value,
				}),
			},
		}
		if conf.APIPodMonitorEnabled {
			curCache[&monitoringv1.PodMonitor{}] = cache.ByObject{
				Label: labels.SelectorFromSet(labels.Set{
					cacheLabel.Name: cacheLabel.Value,
				}),
			}
		}
		if conf.APIVolumeSnapshotEnabled {
			curCache[&storagesnapshotv1.VolumeSnapshot{}] = cache.ByObject{
				Label: labels.SelectorFromSet(labels.Set{
					cacheLabel.Name: cacheLabel.Value,
				}),
			}
		}
		managerOptions.Cache = cache.Options{
			ByObject: curCache,
		}
		setupLog.Info("Listening for changes on all namespaces")
	default:
		setupLog.Info("Operator is Cluster Wide WITHOUT filtered cache !!!")
		setupLog.Info("Listening for changes on all namespaces")
	}

	if conf.WebhookCertDir != "" {
		// If OLM will generate certificates for us, let's just
		// use those
		managerOptions.WebhookServer.(*webhook.DefaultServer).Options.CertDir = conf.WebhookCertDir
	}

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), managerOptions)
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		return err
	}

	webhookServer := mgr.GetWebhookServer().(*webhook.DefaultServer)
	if conf.WebhookCertDir != "" {
		// Use certificate names compatible with OLM
		webhookServer.Options.CertName = "apiserver.crt"
		webhookServer.Options.KeyName = "apiserver.key"
	} else {
		webhookServer.Options.CertName = "tls.crt"
		webhookServer.Options.KeyName = "tls.key"
	}

	// kubeClient is the kubernetes client set with
	// support for the apiextensions that is used
	// during the initialization of the operator
	// kubeClient client.Client
	kubeClient, err := client.New(mgr.GetConfig(), client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "unable to create Kubernetes client")
		return err
	}
	setupLog.Info("Operator configuration loaded", "configuration", conf)

	discoveryClient, err := utils.GetDiscoveryClient()
	if err != nil {
		return err
	}

	// Detect if we are running under a system that implements OpenShift Security Context Constraints
	if err = utils.DetectSecurityContextConstraints(discoveryClient); err != nil {
		setupLog.Error(err, "unable to detect OpenShift Security Context Constraints presence")
		return err
	}

	// Detect if we are running under a system that provides Volume Snapshots
	if err = utils.DetectVolumeSnapshotExist(discoveryClient); err != nil {
		setupLog.Error(err, "unable to detect the if the cluster have the VolumeSnapshot CRD installed")
		return err
	}

	// Detect the available architectures
	if err = utils.DetectAvailableArchitectures(); err != nil {
		setupLog.Error(err, "unable to detect the available instance's architectures")
		return err
	}

	setupLog.Info("Kubernetes system metadata",
		"haveSCC", utils.HaveSecurityContextConstraints(),
		"haveVolumeSnapshot", utils.HaveVolumeSnapshot(),
		"availableArchitectures", utils.GetAvailableArchitectures(),
	)

	if err := ensurePKI(ctx, kubeClient, webhookServer.Options.CertDir, conf); err != nil {
		return err
	}

	pluginRepository := repository.New()
	if _, err := pluginRepository.RegisterUnixSocketPluginsInPath(
		conf.PluginSocketDir,
	); err != nil {
		setupLog.Error(err, "Unable to load sidecar CNPG-i plugins, skipping")
	}

	if err = controller.NewClusterReconciler(
		mgr,
		discoveryClient,
		pluginRepository,
	).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Cluster")
		return err
	}

	if err = controller.NewBackupReconciler(
		mgr,
		discoveryClient,
		pluginRepository,
	).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Backup")
		return err
	}

	if err = controller.NewPluginReconciler(
		mgr,
		pluginRepository,
	).SetupWithManager(mgr, configuration.Current.OperatorNamespace); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Plugin")
		return err
	}

	if err = (&controller.ScheduledBackupReconciler{
		Client:   mgr.GetClient(),
		Scheme:   mgr.GetScheme(),
		Recorder: mgr.GetEventRecorderFor("cloudnative-pg-scheduledbackup"),
	}).SetupWithManager(ctx, mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ScheduledBackup")
		return err
	}

	if err = (&controller.PoolerReconciler{
		Client:          mgr.GetClient(),
		DiscoveryClient: discoveryClient,
		Scheme:          mgr.GetScheme(),
		Recorder:        mgr.GetEventRecorderFor("cloudnative-pg-pooler"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Pooler")
		return err
	}

	if err = (&apiv1.Cluster{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Cluster", "version", "v1")
		return err
	}

	if err = (&apiv1.Backup{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Backup", "version", "v1")
		return err
	}

	if err = (&apiv1.ScheduledBackup{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "ScheduledBackup", "version", "v1")
		return err
	}

	if err = (&apiv1.Pooler{}).SetupWebhookWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create webhook", "webhook", "Pooler", "version", "v1")
		return err
	}

	// Setup the handler used by the readiness and liveliness probe.
	//
	// Unfortunately the readiness of the probe is not sufficient for the operator to be
	// working correctly. The probe may be positive even when:
	//
	// 1. the CA is not yet updated inside the CRD and/or in the validating/mutating
	//    webhook configuration. In that case we have a timeout error after trying
	//    to send a POST message and getting no response message.
	//
	// 2. the webhook service and/or the CNI are being updated, e.g. when a POD is
	//    deleted. In that case we could get a "Connection refused" error message.
	webhookServer.WebhookMux().HandleFunc("/readyz", readinessProbeHandler)

	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		return err
	}

	return nil
}

// loadConfiguration reads the configuration from the provided configmap and secret
func loadConfiguration(
	ctx context.Context,
	kubeClient client.Client,
	configMapName string,
	secretName string,
	conf *configuration.Data,
) error {
	configData := make(map[string]string)

	// First read the configmap if provided and store it in configData
	if configMapName != "" {
		configMapData, err := readConfigMap(ctx, kubeClient, conf.OperatorNamespace, configMapName)
		if err != nil {
			setupLog.Error(err, "unable to read ConfigMap",
				"namespace", conf.OperatorNamespace,
				"name", configMapName)
			return err
		}
		for k, v := range configMapData {
			configData[k] = v
		}
	}

	// Then read the secret if provided and store it in configData, overwriting configmap's values
	if secretName != "" {
		secretData, err := readSecret(ctx, kubeClient, conf.OperatorNamespace, secretName)
		if err != nil {
			setupLog.Error(err, "unable to read Secret",
				"namespace", conf.OperatorNamespace,
				"name", secretName)
			return err
		}
		for k, v := range secretData {
			configData[k] = v
		}
	}

	// Finally, read the config if it was provided
	if len(configData) > 0 {
		conf.ReadConfigMap(configData)
	}

	// Overwrite some parameters at runtime

	// Check, if the API Node is enabled, we can list nodes, otherwise we disable the API
	if configuration.Current.APINodeEnabled {
		node := &corev1.Node{}
		var nodeList client.ObjectList
		nodeSelector := client.MatchingFields{
			"involvedObject.apiVersion": node.APIVersion,
			"involvedObject.kind":       node.Kind,
			"involvedObject.namespace":  configuration.Current.OperatorNamespace,
		}
		err := kubeClient.List(ctx, nodeList, nodeSelector)
		if err != nil {
			configuration.Current.DisableNodeAPI()
		}
	}

	// Check, if the API PodMonitor is enabled, CRD exists, otherwise we disable the API
	discoveryClient, err := utils.GetDiscoveryClient()
	if err != nil {
		return err
	}

	ape, err := utils.PodMonitorExist(discoveryClient)
	if err != nil || !ape {
		configuration.Current.DisablePodMonitor()
	}

	vse, err := utils.VolumeSnapshotExist(discoveryClient)
	if err != nil || !vse {
		configuration.Current.DisableVolumeSnapshot()
	}

	// Check, if the API PodMonitor is enabled, we can list PodMonitors inside Operator own namespace
	if configuration.Current.APIPodMonitorEnabled {
		podMonitor := &monitoringv1.PodMonitor{}
		var podList client.ObjectList
		podSelector := client.MatchingFields{
			"involvedObject.apiVersion": podMonitor.APIVersion,
			"involvedObject.kind":       podMonitor.Kind,
			"involvedObject.namespace":  configuration.Current.OperatorNamespace,
		}
		err := kubeClient.List(ctx, podList, podSelector)
		if err != nil {
			configuration.Current.DisablePodMonitor()
		}
	}

	// Check if the API ClusterImageCatalog is enabled, we can list ClusterImageCatalog inside Operator own namespace
	if configuration.Current.APIClusterImageCatalogEnabled {
		cic := &apiv1.ClusterImageCatalog{}
		var cicList client.ObjectList
		cicSelector := client.MatchingFields{
			"involvedObject.apiVersion": cic.APIVersion,
			"involvedObject.kind":       cic.Kind,
			"involvedObject.namespace":  configuration.Current.OperatorNamespace,
		}
		err := kubeClient.List(ctx, cicList, cicSelector)
		if err != nil {
			configuration.Current.DisableClusterImageCatalog()
		}
	}

	// Check if the API VolumeSnapshot is enabled, we can list VolumeSnapshot inside Operator own namespace
	if configuration.Current.APIVolumeSnapshotEnabled {
		vs := &storagesnapshotv1.VolumeSnapshot{}
		var vsList client.ObjectList
		vsSelector := client.MatchingFields{
			"involvedObject.apiVersion": vs.APIVersion,
			"involvedObject.kind":       vs.Kind,
			"involvedObject.namespace":  configuration.Current.OperatorNamespace,
		}
		err := kubeClient.List(ctx, vsList, vsSelector)
		if err != nil {
			configuration.Current.DisableVolumeSnapshot()
		}
	}

	return nil
}

// readinessProbeHandler is used to implement the readiness probe handler
func readinessProbeHandler(w http.ResponseWriter, _ *http.Request) {
	_, _ = fmt.Fprint(w, "OK")
}

// ensurePKI ensures that we have the required PKI infrastructure to make
// the operator and the clusters working
func ensurePKI(
	ctx context.Context,
	kubeClient client.Client,
	mgrCertDir string,
	conf *configuration.Data,
) error {
	if conf.WebhookCertDir != "" {
		// OLM is generating certificates for us, so we can avoid injecting/creating certificates.
		return nil
	}

	// We need to self-manage required PKI infrastructure and install the certificates into
	// the webhooks configuration
	pkiConfig := certs.PublicKeyInfrastructure{
		CaSecretName:                       conf.CaSecretName,
		CertDir:                            mgrCertDir,
		SecretName:                         conf.WebhookSecretName,
		ServiceName:                        conf.WebhookServiceName,
		OperatorNamespace:                  conf.OperatorNamespace,
		MutatingWebhookConfigurationName:   conf.MutatingWebhookName,
		ValidatingWebhookConfigurationName: conf.ValidatingWebhookName,
		OperatorDeploymentLabelSelector:    conf.OperatorSelector,
	}
	err := pkiConfig.Setup(ctx, kubeClient)
	if err != nil {
		setupLog.Error(err, "unable to setup PKI infrastructure")
	}
	return err
}

// readConfigMap reads the configMap and returns its content as map
func readConfigMap(
	ctx context.Context,
	kubeClient client.Client,
	namespace string,
	name string,
) (map[string]string, error) {
	if name == "" {
		return nil, nil
	}

	if namespace == "" {
		return nil, nil
	}

	setupLog.Info("Loading configuration from ConfigMap",
		"namespace", namespace,
		"name", name)

	configMap := &corev1.ConfigMap{}
	err := kubeClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, configMap)
	if apierrs.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return configMap.Data, nil
}

// readSecret reads the secret and returns its content as map
func readSecret(
	ctx context.Context,
	kubeClient client.Client,
	namespace,
	name string,
) (map[string]string, error) {
	if name == "" {
		return nil, nil
	}

	if namespace == "" {
		return nil, nil
	}

	setupLog.Info("Loading configuration from Secret",
		"namespace", namespace,
		"name", name)

	secret := &corev1.Secret{}
	err := kubeClient.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, secret)
	if apierrs.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	data := make(map[string]string)
	for k, v := range secret.Data {
		data[k] = string(v)
	}

	return data, nil
}

// startPprofDebugServer exposes pprof debug server if the pprof-server env variable is set to true
func startPprofDebugServer(ctx context.Context) {
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	pprofServer := http.Server{
		Addr:              "0.0.0.0:6060",
		Handler:           mux,
		ReadTimeout:       webserver.DefaultReadTimeout,
		ReadHeaderTimeout: webserver.DefaultReadHeaderTimeout,
	}

	setupLog.Info("Starting pprof HTTP server", "addr", pprofServer.Addr)

	go func() {
		go func() {
			<-ctx.Done()

			setupLog.Info("shutting down pprof HTTP server")
			ctx, cancelFunc := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancelFunc()

			if err := pprofServer.Shutdown(ctx); err != nil {
				setupLog.Error(err, "Failed to shutdown pprof HTTP server")
			}
		}()

		if err := pprofServer.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			setupLog.Error(err, "Failed to start pprof HTTP server")
		}
	}()
}

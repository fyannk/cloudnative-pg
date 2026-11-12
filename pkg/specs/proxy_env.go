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
	"fmt"
	"net/url"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"
)

// proxyEnvironmentNames are the standard proxy settings, in the upper case
// spelling that Go reads first. The lower case spelling is read as a fallback,
// and both spellings are set on the workloads: curl and some Python tooling
// read the lower case one only.
var proxyEnvironmentNames = []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY"}

// ProxySetting is one proxy setting of the operator, as found in its environment
type ProxySetting struct {
	// Name is the upper case name of the setting
	Name string

	// Value is the value found in the environment of the operator
	Value string
}

// OperatorProxySettings reads the proxy settings from the environment of the
// operator, in the order Go reads them: the upper case name first, the lower
// case one as a fallback. Settings with an empty value are left out.
func OperatorProxySettings() []ProxySetting {
	return operatorProxySettings(os.LookupEnv)
}

func operatorProxySettings(lookup func(string) (string, bool)) []ProxySetting {
	result := make([]ProxySetting, 0, len(proxyEnvironmentNames))
	for _, name := range proxyEnvironmentNames {
		value, _ := lookup(name)
		if value == "" {
			value, _ = lookup(strings.ToLower(name))
		}
		if value == "" {
			continue
		}
		result = append(result, ProxySetting{Name: name, Value: value})
	}
	return result
}

// ProxyEnvironment returns the proxy settings of the operator as environment
// variables for a workload, in both spellings, when the operator is configured
// to propagate them. When the workload defines any of the proxy variables
// itself, in any case, nothing is returned: the workload is in control of its
// proxy settings as a whole.
func ProxyEnvironment(workloadEnv []corev1.EnvVar) []corev1.EnvVar {
	if !configuration.Current.PropagateProxyEnv {
		return nil
	}

	return proxyEnvironment(OperatorProxySettings(), workloadEnv)
}

func proxyEnvironment(settings []ProxySetting, workloadEnv []corev1.EnvVar) []corev1.EnvVar {
	if len(settings) == 0 {
		return nil
	}

	for _, env := range workloadEnv {
		if IsProxyEnvironmentName(env.Name) {
			return nil
		}
	}

	result := make([]corev1.EnvVar, 0, 2*len(settings))
	for _, setting := range settings {
		result = append(result,
			corev1.EnvVar{Name: setting.Name, Value: setting.Value},
			corev1.EnvVar{Name: strings.ToLower(setting.Name), Value: setting.Value},
		)
	}

	return result
}

// IsProxyEnvironmentName tells whether name is one of the proxy settings,
// whatever its case
func IsProxyEnvironmentName(name string) bool {
	upper := strings.ToUpper(name)
	for _, proxyName := range proxyEnvironmentNames {
		if upper == proxyName {
			return true
		}
	}

	return false
}

// IsURL tells whether the setting holds a proxy URL. NO_PROXY holds a list
// of hosts instead.
func (setting ProxySetting) IsURL() bool {
	return setting.Name != "NO_PROXY"
}

// Validate returns an error when the setting holds a proxy URL that Go
// would not accept
func (setting ProxySetting) Validate() error {
	if !setting.IsURL() {
		return nil
	}

	_, err := parseProxyURL(setting.Value)
	return err
}

// Redacted returns the setting as it can be logged: the credentials of a
// proxy URL are replaced, and a value that is not a valid URL is hidden
func (setting ProxySetting) Redacted() string {
	if !setting.IsURL() {
		return setting.Value
	}

	proxyURL, err := parseProxyURL(setting.Value)
	if err != nil {
		return "(not a valid URL)"
	}
	if proxyURL.User != nil {
		proxyURL.User = url.User("REDACTED")
	}

	return proxyURL.String()
}

// parseProxyURL parses a proxy URL the way Go's http.ProxyFromEnvironment
// does: a value without a scheme is read as an http URL
func parseProxyURL(value string) (*url.URL, error) {
	proxyURL, err := url.Parse(value)
	if err != nil || proxyURL.Scheme == "" || proxyURL.Host == "" {
		if withScheme, errWithScheme := url.Parse("http://" + value); errWithScheme == nil {
			return withScheme, nil
		}
	}
	if err != nil {
		return nil, fmt.Errorf("invalid proxy address: %w", err)
	}

	return proxyURL, nil
}

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

package operatormetrics

import (
	"github.com/cloudnative-pg/cloudnative-pg/internal/configuration"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// One scrape of a configuration as the operator holds it after startup: every
// setting is reported by its environment variable with its effective value,
// lists comma-separated, an unset optional value empty, an API disabled by
// the startup probe as false, and the settings read from the Secret redacted.
var _ = Describe("The operator configuration metrics", func() {
	It("reports every setting with its effective value and redacts those from the Secret", func() {
		conf := configuration.NewConfiguration()
		conf.WatchNamespace = "tenant-a,tenant-b"
		conf.InheritedLabels = []string{"team", "env"}
		conf.APINodeEnabled = false
		conf.OperatorPullSecretName = "registry-token"

		series := gather(NewConfigCollector(conf, []string{"PULL_SECRET_NAME"}))
		setting := func(name, value string) string {
			return `cnpg_operator_config_info{name="` + name + `",value="` + value + `"}`
		}

		Expect(series).To(HaveKeyWithValue(setting("WATCH_NAMESPACE", "tenant-a,tenant-b"), 1.0))
		Expect(series).To(HaveKeyWithValue(setting("INHERITED_LABELS", "team,env"), 1.0))
		Expect(series).To(HaveKeyWithValue(setting("API_NODE_ENABLED", "false"), 1.0))
		Expect(series).To(HaveKeyWithValue(setting("API_POD_MONITOR_ENABLED", "true"), 1.0))
		Expect(series).To(HaveKeyWithValue(setting("STANDBY_TCP_USER_TIMEOUT", ""), 1.0))
		Expect(series).To(HaveKeyWithValue(setting("PULL_SECRET_NAME", "<redacted>"), 1.0))
		Expect(series).ToNot(HaveKey(ContainSubstring("registry-token")))
	})
})

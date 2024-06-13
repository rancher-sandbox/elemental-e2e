/*
Copyright © 2022 - 2024 SUSE LLC

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

package e2e_test

import (
	"os"

	. "github.com/onsi/ginkgo/v2"
	"github.com/rancher-sandbox/ele-testhelpers/kubectl"
)

func checkRC(err error) {
	if err != nil {
		GinkgoWriter.Printf("%s\n", err)
	}
}

var _ = Describe("E2E - Getting logs node", Label("logs"), func() {
	type binary struct {
		Url  string
		Name string
	}

	type getResourceLog struct {
		Name string
		Verb []string
	}

	It("Get the upstream cluster logs", func() {
		// Report to Qase
		testCaseID = 69

		By("Collecting additionals logs with kubectl commands", func() {
			Bundles := getResourceLog{
				"bundles",
				[]string{"get", "describe"},
			}

			var getResources []getResourceLog = []getResourceLog{Bundles}
			for _, r := range getResources {
				for _, v := range r.Verb {
					outcmd, err := kubectl.RunWithoutErr(v, r.Name, "--all-namespaces")
					checkRC(err)
					err = os.WriteFile(r.Name+"-"+v+".log", []byte(outcmd), os.ModePerm)
					checkRC(err)
				}
			}
		})
	})
})

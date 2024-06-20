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
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/rancher-sandbox/ele-testhelpers/tools"
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

	It("Get the cluster logs", func() {
		// Report to Qase
		testCaseID = 69

		By("Install crush-gather tool", func() {
			crustGather := binary{
				"https://github.com/crust-gather/crust-gather/raw/main/install.sh",
				"crust-gather-installer",
			}

			_ = os.Mkdir("logs", 0755)
			_ = os.Chdir("logs")
			myDir, _ := os.Getwd()

			for _, b := range []binary{crustGather} {
				Eventually(func() error {
					return exec.Command("curl", "-L", b.Url, "-o", b.Name).Run()
				}, tools.SetTimeout(1*time.Minute), 5*time.Second).Should(BeNil())

				err := exec.Command("chmod", "+x", b.Name).Run()
				checkRC(err)
				err = exec.Command("sudo", myDir+"/"+b.Name, "-f", "-y").Run()
				checkRC(err)
				err = exec.Command("crust-gather", "collect").Run()
				checkRC(err)
			}
		})
	})
})

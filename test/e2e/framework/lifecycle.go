/*
Copyright 2024.

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

package framework

import (
	"time"

	. "github.com/onsi/ginkgo/v2"

	"github.com/kaasops/vector-operator/test/utils"
)

// SharedDependencies manages dependencies that are shared across all tests
type SharedDependencies struct {
	prometheusInstalled  bool
	certManagerInstalled bool
	installTime          time.Duration
}

var globalDeps *SharedDependencies

// InstallSharedDependencies installs Prometheus and cert-manager once for all tests
// This should be called in BeforeSuite
func InstallSharedDependencies() {
	if globalDeps != nil {
		GinkgoWriter.Println("⚠️  Shared dependencies already installed, skipping...")
		return
	}

	start := time.Now()
	globalDeps = &SharedDependencies{}

	By("installing Prometheus Operator (shared)")
	err := utils.InstallPrometheusOperator()
	if err != nil {
		// Ignore AlreadyExists errors - dependencies might be already installed
		GinkgoWriter.Printf("⚠️  Prometheus Operator installation returned error (might already exist): %v\n", err)
	}
	globalDeps.prometheusInstalled = true

	By("installing cert-manager (shared)")
	err = utils.InstallCertManager()
	if err != nil {
		// Ignore AlreadyExists errors - dependencies might be already installed
		GinkgoWriter.Printf("⚠️  cert-manager installation returned error (might already exist): %v\n", err)
	}
	globalDeps.certManagerInstalled = true

	globalDeps.installTime = time.Since(start)
	GinkgoWriter.Printf("✅ Shared dependencies installed in %v\n", globalDeps.installTime)
}

// UninstallSharedDependencies removes Prometheus and cert-manager
// This should be called in AfterSuite
func UninstallSharedDependencies() {
	if globalDeps == nil {
		return
	}

	By("uninstalling Prometheus Operator (shared)")
	if globalDeps.prometheusInstalled {
		utils.UninstallPrometheusOperator()
	}

	By("uninstalling cert-manager (shared)")
	if globalDeps.certManagerInstalled {
		utils.UninstallCertManager()
	}

	GinkgoWriter.Println("✅ Shared dependencies uninstalled")
	globalDeps = nil
}

// AreSharedDependenciesInstalled checks if shared dependencies are available
func AreSharedDependenciesInstalled() bool {
	return globalDeps != nil && globalDeps.prometheusInstalled && globalDeps.certManagerInstalled
}

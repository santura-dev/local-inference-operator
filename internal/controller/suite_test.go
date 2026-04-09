// Copyright 2025 Sandra Poturalska
// SPDX-License-Identifier: MIT

package controller

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	servingv1 "github.com/santura-dev/local-inference-operator/api/v1"
	// +kubebuilder:scaffold:imports
)

var (
	ctx       context.Context
	cancel    context.CancelFunc
	testEnv   *envtest.Environment
	cfg       *rest.Config
	k8sClient client.Client
)

func TestControllers(t *testing.T) {
	RegisterFailHandler(Fail)

	RunSpecs(t, "Controller Suite")
}

var _ = BeforeSuite(func() {
	assets := getFirstFoundEnvTestBinaryDir()
	if os.Getenv("KUBEBUILDER_ASSETS") == "" && assets == "" {
		Skip("envtest binaries not found. Run 'make setup-envtest' or 'make test' to install them.")
	}

	logf.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	ctx, cancel = context.WithCancel(context.TODO())

	var err error
	err = servingv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}
	if assets != "" {
		testEnv.BinaryAssetsDirectory = assets
	}

	cfg, err = testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())
})

var _ = AfterSuite(func() {
	if testEnv == nil {
		return
	}
	By("tearing down the test environment")
	if cancel != nil {
		cancel()
	}
	Expect(testEnv.Stop()).To(Succeed())
})

// getFirstFoundEnvTestBinaryDir locates the first binary directory installed by
// setup-envtest so IDE runs work without KUBEBUILDER_ASSETS being set.
func getFirstFoundEnvTestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}

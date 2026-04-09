// Copyright 2025 Sandra Poturalska
// SPDX-License-Identifier: MIT

package controller

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	servingv1 "github.com/santura-dev/local-inference-operator/api/v1"
)

var _ = Describe("LocalInferenceService Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			resource := &servingv1.LocalInferenceService{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: servingv1.LocalInferenceServiceSpec{
					Runtime: "vllm",
					Model: servingv1.ModelSpec{
						URI:  "hf://facebook/opt-125m",
						Name: "OPT 125M",
					},
					Settings: servingv1.SettingsSpec{
						Precision: "fp16",
						BatchSize: 1,
						MaxTokens: 100,
					},
					Scaling: servingv1.ScalingSpec{Replicas: 1},
				},
			}
			err := k8sClient.Create(ctx, resource)
			if err != nil && !apierrors.IsAlreadyExists(err) {
				Expect(err).NotTo(HaveOccurred())
			}
		})

		AfterEach(func() {
			resource := &servingv1.LocalInferenceService{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if err == nil {
				Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
			}
		})

		It("creates an owned Deployment and Service with health probes", func() {
			controllerReconciler := &LocalInferenceServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			deployment := &appsv1.Deployment{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-deployment",
				Namespace: "default",
			}, deployment)).To(Succeed())

			Expect(deployment.OwnerReferences).To(HaveLen(1))
			Expect(deployment.OwnerReferences[0].Kind).To(Equal("LocalInferenceService"))
			Expect(deployment.Spec.Template.Spec.Containers).To(HaveLen(1))

			container := deployment.Spec.Template.Spec.Containers[0]
			Expect(container.ReadinessProbe).NotTo(BeNil())
			Expect(container.ReadinessProbe.HTTPGet.Path).To(Equal("/health"))
			Expect(container.LivenessProbe).NotTo(BeNil())

			service := &corev1.Service{}
			Expect(k8sClient.Get(ctx, types.NamespacedName{
				Name:      resourceName + "-service",
				Namespace: "default",
			}, service)).To(Succeed())
			Expect(service.OwnerReferences).To(HaveLen(1))
		})

		It("reports a status phase based on the Deployment state", func() {
			controllerReconciler := &LocalInferenceServiceReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &servingv1.LocalInferenceService{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
			Expect(updated.Status.Phase).To(Equal("Progressing"))
			Expect(updated.Status.DeploymentName).To(Equal(resourceName + "-deployment"))
			Expect(updated.Status.Conditions).NotTo(BeEmpty())
		})
	})
})

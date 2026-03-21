/*
Copyright 2025.

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

package controller

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	servingv1 "local-ome/api/v1"
)

// LocalInferenceServiceReconciler reconciles a LocalInferenceService object
type LocalInferenceServiceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=serving.local-ome.com,resources=localinferenceservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=serving.local-ome.com,resources=localinferenceservices/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=serving.local-ome.com,resources=localinferenceservices/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// The Reconcile function compares the state specified by the LocalInferenceService
// object against the actual cluster state, and performs operations to make the
// cluster state reflect the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.22.4/pkg/reconcile
func (r *LocalInferenceServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// Fetch the LocalInferenceService instance
	var lis servingv1.LocalInferenceService
	if err := r.Get(ctx, req.NamespacedName, &lis); err != nil {
		if errors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			log.Info("LocalInferenceService resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		log.Error(err, "Failed to get LocalInferenceService")
		return ctrl.Result{}, err
	}

	// Determine the container image based on runtime
	image := r.getImageForRuntime(lis.Spec.Runtime)

	// Prepare environment variables from spec
	envVars := r.buildEnvVars(&lis.Spec)

	// Create or update the Deployment
	deploymentName := fmt.Sprintf("%s-deployment", lis.Name)
	serviceName := fmt.Sprintf("%s-service", lis.Name)

	if err := r.ensureDeployment(ctx, &lis, deploymentName, image, envVars); err != nil {
		log.Error(err, "Failed to ensure Deployment")
		return ctrl.Result{}, err
	}

	// Create or update the Service
	if err := r.ensureService(ctx, &lis, serviceName, deploymentName); err != nil {
		log.Error(err, "Failed to ensure Service")
		return ctrl.Result{}, err
	}

	// Create or update HPA if auto-scaling is enabled
	if lis.Spec.Scaling.AutoScale {
		hpaName := fmt.Sprintf("%s-hpa", lis.Name)
		if err := r.ensureHPA(ctx, &lis, hpaName, deploymentName); err != nil {
			log.Error(err, "Failed to ensure HPA")
			return ctrl.Result{}, err
		}
	}

	// Update status
	lis.Status.Phase = "Running"
	lis.Status.DeploymentName = deploymentName
	lis.Status.ServiceName = serviceName
	lis.Status.ReadyReplicas = lis.Spec.Scaling.Replicas
	if err := r.Status().Update(ctx, &lis); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled LocalInferenceService", "name", lis.Name)
	return ctrl.Result{}, nil
}

// getImageForRuntime returns the appropriate container image based on the runtime
func (r *LocalInferenceServiceReconciler) getImageForRuntime(runtime string) string {
	switch strings.ToLower(runtime) {
	case "vllm":
		return "local-vllm:latest"
	case "sglang":
		return "lmsysorg/sglang:latest"
	default:
		return "local-vllm:latest" // Default to vLLM
	}
}

// buildEnvVars constructs environment variables from the spec
func (r *LocalInferenceServiceReconciler) buildEnvVars(spec *servingv1.LocalInferenceServiceSpec) []corev1.EnvVar {
	envVars := []corev1.EnvVar{
		{
			Name:  "MODEL_URI",
			Value: spec.Model.URI,
		},
	}

	if spec.Model.Name != "" {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "MODEL_NAME",
			Value: spec.Model.Name,
		})
	}

	return envVars
}

// buildCommand constructs the command array based on runtime
func (r *LocalInferenceServiceReconciler) buildCommand(runtime string, spec servingv1.LocalInferenceServiceSpec) []string {
	return []string{
		"python", "-m", "sglang.launch_server",
		"--model", spec.Model.URI,
		"--mem-fraction", "0.8",
		"--dtype", r.mapPrecision(spec.Settings.Precision),
		"--attention-backend", "flashinfer",
		"--host", "0.0.0.0",
		"--port", "8000",
	}
}

// mapPrecision maps CRD precision to vLLM dtype
func (r *LocalInferenceServiceReconciler) mapPrecision(precision string) string {
	switch precision {
	case "fp16":
		return "float16"
	case "fp32":
		return "float32"
	case "int8":
		return "int8"
	default:
		return "float16"
	}
}

// ensureDeployment creates or updates the Deployment for the inference service
func (r *LocalInferenceServiceReconciler) ensureDeployment(ctx context.Context, lis *servingv1.LocalInferenceService, name, image string, envVars []corev1.EnvVar) error {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: lis.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &lis.Spec.Scaling.Replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": lis.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": lis.Name,
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "inference",
							Image: image,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: 8000,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env:     envVars,
							Command: r.buildCommand(lis.Spec.Runtime, lis.Spec),
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
									"nvidia.com/gpu":      resource.MustParse("1"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("8Gi"),
									"nvidia.com/gpu":      resource.MustParse("1"),
								},
							},
						},
					},
				},
			},
		},
	}

	// Try to create; if it exists, update
	if err := r.Create(ctx, dep); err != nil {
		if errors.IsAlreadyExists(err) {
			// Update existing deployment
			existing := &appsv1.Deployment{}
			if err := r.Get(ctx, client.ObjectKeyFromObject(dep), existing); err != nil {
				return err
			}
			existing.Spec = dep.Spec
			return r.Update(ctx, existing)
		}
		return err
	}
	return nil
}

// ensureHPA creates or updates the HorizontalPodAutoscaler for auto-scaling
func (r *LocalInferenceServiceReconciler) ensureHPA(ctx context.Context, lis *servingv1.LocalInferenceService, name, deploymentName string) error {
	defaultCPUUtilization := int32(70)

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: lis.Namespace,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deploymentName,
			},
			MinReplicas: &lis.Spec.Scaling.MinReplicas,
			MaxReplicas: lis.Spec.Scaling.MaxReplicas,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: &defaultCPUUtilization,
						},
					},
				},
			},
		},
	}

	if err := r.Create(ctx, hpa); err != nil {
		if errors.IsAlreadyExists(err) {
			existing := &autoscalingv2.HorizontalPodAutoscaler{}
			if err := r.Get(ctx, client.ObjectKeyFromObject(hpa), existing); err != nil {
				return err
			}
			existing.Spec = hpa.Spec
			return r.Update(ctx, existing)
		}
		return err
	}
	return nil
}

// ensureService creates or updates the Service for the inference service
func (r *LocalInferenceServiceReconciler) ensureService(ctx context.Context, lis *servingv1.LocalInferenceService, name, deploymentName string) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: lis.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": lis.Name,
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8000),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	// Try to create; if it exists, update
	if err := r.Create(ctx, svc); err != nil {
		if errors.IsAlreadyExists(err) {
			// Update existing service
			existing := &corev1.Service{}
			if err := r.Get(ctx, client.ObjectKeyFromObject(svc), existing); err != nil {
				return err
			}
			existing.Spec = svc.Spec
			return r.Update(ctx, existing)
		}
		return err
	}
	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LocalInferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&servingv1.LocalInferenceService{}).
		Named("localinferenceservice").
		Complete(r)
}

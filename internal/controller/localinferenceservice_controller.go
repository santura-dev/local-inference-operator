// Copyright 2025 Sandra Poturalska
// SPDX-License-Identifier: MIT

package controller

import (
	"context"
	"fmt"
	"os"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	servingv1 "github.com/santura-dev/local-inference-operator/api/v1"
)

const (
	inferencePort   = int32(8000)
	gpuResourceName = "nvidia.com/gpu"

	defaultVLLMImage   = "vllm/vllm-openai:latest"
	defaultSGLangImage = "lmsysorg/sglang:latest"
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

func (r *LocalInferenceServiceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	var lis servingv1.LocalInferenceService
	if err := r.Get(ctx, req.NamespacedName, &lis); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("LocalInferenceService resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get LocalInferenceService")
		return ctrl.Result{}, err
	}

	deploymentName := fmt.Sprintf("%s-deployment", lis.Name)
	serviceName := fmt.Sprintf("%s-service", lis.Name)
	hpaName := fmt.Sprintf("%s-hpa", lis.Name)

	if err := r.ensureDeployment(ctx, &lis, deploymentName); err != nil {
		log.Error(err, "Failed to ensure Deployment")
		return ctrl.Result{}, err
	}

	if err := r.ensureService(ctx, &lis, serviceName); err != nil {
		log.Error(err, "Failed to ensure Service")
		return ctrl.Result{}, err
	}

	if lis.Spec.Scaling.AutoScale {
		if err := r.ensureHPA(ctx, &lis, hpaName, deploymentName); err != nil {
			log.Error(err, "Failed to ensure HPA")
			return ctrl.Result{}, err
		}
	} else if err := r.deleteHPAIfPresent(ctx, hpaName, lis.Namespace); err != nil {
		log.Error(err, "Failed to remove HPA")
		return ctrl.Result{}, err
	}

	if err := r.updateStatus(ctx, &lis, deploymentName, serviceName); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	log.Info("Successfully reconciled LocalInferenceService", "name", lis.Name, "phase", lis.Status.Phase)
	return ctrl.Result{}, nil
}

// imageForRuntime resolves public runtime images, overridable per environment.
func imageForRuntime(runtimeName string) string {
	if strings.ToLower(runtimeName) == "sglang" {
		if image := os.Getenv("RELATED_IMAGE_SGLANG"); image != "" {
			return image
		}
		return defaultSGLangImage
	}
	if image := os.Getenv("RELATED_IMAGE_VLLM"); image != "" {
		return image
	}
	return defaultVLLMImage
}

// commandForRuntime builds the serving command for the selected runtime. The two
// runtimes take different flags, so the command has to follow the runtime.
func commandForRuntime(runtimeName string, spec servingv1.LocalInferenceServiceSpec) []string {
	dtype := mapPrecision(spec.Settings.Precision)

	if strings.ToLower(runtimeName) == "sglang" {
		args := []string{
			"python", "-m", "sglang.launch_server",
			"--model-path", spec.Model.URI,
			"--host", "0.0.0.0",
			"--port", fmt.Sprint(inferencePort),
			"--dtype", dtype,
			"--mem-fraction-static", "0.9",
		}
		if spec.Settings.BatchSize > 0 {
			args = append(args, "--max-running-requests", fmt.Sprint(spec.Settings.BatchSize))
		}
		return args
	}

	args := []string{
		"vllm", "serve", spec.Model.URI,
		"--host", "0.0.0.0",
		"--port", fmt.Sprint(inferencePort),
		"--dtype", dtype,
		"--gpu-memory-utilization", "0.9",
	}
	if spec.Settings.BatchSize > 0 {
		args = append(args, "--max-num-seqs", fmt.Sprint(spec.Settings.BatchSize))
	}
	return args
}

func mapPrecision(precision string) string {
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

// replicasFor defaults to one replica when the CRD default has not been applied,
// which is the case in unit tests that bypass the API server.
func replicasFor(spec *servingv1.LocalInferenceServiceSpec) int32 {
	if spec.Scaling.Replicas > 0 {
		return spec.Scaling.Replicas
	}
	return 1
}

// memoryLimitFor maps settings.gpuMemory onto the container memory limit.
// Kubernetes schedules GPU count, not GPU memory, so this is the closest
// schedulable signal for the requested model footprint.
func memoryLimitFor(spec *servingv1.LocalInferenceServiceSpec) resource.Quantity {
	if spec.Settings.GPUMemory != "" {
		if quantity, err := resource.ParseQuantity(spec.Settings.GPUMemory); err == nil {
			return quantity
		}
	}
	return resource.MustParse("8Gi")
}

func resourcesFor(spec *servingv1.LocalInferenceServiceSpec) corev1.ResourceRequirements {
	return corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
			gpuResourceName:       resource.MustParse("1"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: memoryLimitFor(spec),
			gpuResourceName:       resource.MustParse("1"),
		},
	}
}

func buildEnvVars(spec *servingv1.LocalInferenceServiceSpec) []corev1.EnvVar {
	envVars := []corev1.EnvVar{{Name: "MODEL_URI", Value: spec.Model.URI}}
	if spec.Model.Name != "" {
		envVars = append(envVars, corev1.EnvVar{Name: "MODEL_NAME", Value: spec.Model.Name})
	}
	return envVars
}

func healthProbe(initialDelay int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/health",
				Port: intstr.FromInt32(inferencePort),
			},
		},
		InitialDelaySeconds: initialDelay,
		PeriodSeconds:       10,
		FailureThreshold:    3,
	}
}

func selectorLabels(name string) map[string]string {
	return map[string]string{"app": name}
}

func (r *LocalInferenceServiceReconciler) ensureDeployment(ctx context.Context, lis *servingv1.LocalInferenceService, name string) error {
	replicas := replicasFor(&lis.Spec)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: lis.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: selectorLabels(lis.Name)},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: selectorLabels(lis.Name)},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:           "inference",
							Image:          imageForRuntime(lis.Spec.Runtime),
							Command:        commandForRuntime(lis.Spec.Runtime, lis.Spec),
							Env:            buildEnvVars(&lis.Spec),
							Ports:          []corev1.ContainerPort{{ContainerPort: inferencePort, Protocol: corev1.ProtocolTCP}},
							Resources:      resourcesFor(&lis.Spec),
							ReadinessProbe: healthProbe(10),
							LivenessProbe:  healthProbe(30),
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(lis, dep, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, dep); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		existing := &appsv1.Deployment{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(dep), existing); err != nil {
			return err
		}
		existing.Spec = dep.Spec
		if err := controllerutil.SetControllerReference(lis, existing, r.Scheme); err != nil {
			return err
		}
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *LocalInferenceServiceReconciler) ensureService(ctx context.Context, lis *servingv1.LocalInferenceService, name string) error {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: lis.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: selectorLabels(lis.Name),
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt32(inferencePort),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	if err := controllerutil.SetControllerReference(lis, svc, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, svc); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		existing := &corev1.Service{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(svc), existing); err != nil {
			return err
		}
		existing.Spec = svc.Spec
		if err := controllerutil.SetControllerReference(lis, existing, r.Scheme); err != nil {
			return err
		}
		return r.Update(ctx, existing)
	}
	return nil
}

func (r *LocalInferenceServiceReconciler) ensureHPA(ctx context.Context, lis *servingv1.LocalInferenceService, name, deploymentName string) error {
	targetCPU := lis.Spec.Scaling.TargetCPU
	if targetCPU <= 0 {
		targetCPU = 70
	}
	minReplicas := lis.Spec.Scaling.MinReplicas
	if minReplicas <= 0 {
		minReplicas = 1
	}
	maxReplicas := lis.Spec.Scaling.MaxReplicas
	if maxReplicas <= 0 {
		maxReplicas = 10
	}

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
			MinReplicas: &minReplicas,
			MaxReplicas: maxReplicas,
			Metrics: []autoscalingv2.MetricSpec{
				{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: &targetCPU,
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(lis, hpa, r.Scheme); err != nil {
		return err
	}

	if err := r.Create(ctx, hpa); err != nil {
		if !apierrors.IsAlreadyExists(err) {
			return err
		}
		existing := &autoscalingv2.HorizontalPodAutoscaler{}
		if err := r.Get(ctx, client.ObjectKeyFromObject(hpa), existing); err != nil {
			return err
		}
		existing.Spec = hpa.Spec
		if err := controllerutil.SetControllerReference(lis, existing, r.Scheme); err != nil {
			return err
		}
		return r.Update(ctx, existing)
	}
	return nil
}

// deleteHPAIfPresent removes a leftover HPA when autoscaling is turned off.
func (r *LocalInferenceServiceReconciler) deleteHPAIfPresent(ctx context.Context, name, namespace string) error {
	existing := &autoscalingv2.HorizontalPodAutoscaler{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, existing)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return err
	}
	return r.Delete(ctx, existing)
}

// updateStatus reports the real Deployment state instead of assuming readiness.
func (r *LocalInferenceServiceReconciler) updateStatus(ctx context.Context, lis *servingv1.LocalInferenceService, deploymentName, serviceName string) error {
	desired := replicasFor(&lis.Spec)
	phase := "Progressing"
	available := metav1.ConditionFalse
	reason := "DeploymentNotReady"
	message := "Waiting for the inference Deployment to become ready"

	dep := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deploymentName, Namespace: lis.Namespace}, dep)
	if err == nil {
		lis.Status.ReadyReplicas = dep.Status.ReadyReplicas
		if dep.Status.ObservedGeneration >= dep.Generation && dep.Status.ReadyReplicas >= desired {
			phase = "Ready"
			available = metav1.ConditionTrue
			reason = "DeploymentReady"
			message = "The inference Deployment is serving"
		}
	} else if !apierrors.IsNotFound(err) {
		return err
	}

	lis.Status.Phase = phase
	lis.Status.DeploymentName = deploymentName
	lis.Status.ServiceName = serviceName

	meta.SetStatusCondition(&lis.Status.Conditions, metav1.Condition{
		Type:               "Available",
		Status:             available,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: lis.Generation,
	})

	return r.Status().Update(ctx, lis)
}

func (r *LocalInferenceServiceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&servingv1.LocalInferenceService{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Named("localinferenceservice").
		Complete(r)
}

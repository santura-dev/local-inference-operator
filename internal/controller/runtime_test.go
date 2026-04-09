// Copyright 2025 Sandra Poturalska
// SPDX-License-Identifier: MIT

package controller

import (
	"os"
	"strings"
	"testing"

	servingv1 "github.com/santura-dev/local-inference-operator/api/v1"
)

func spec(runtime, uri string) servingv1.LocalInferenceServiceSpec {
	return servingv1.LocalInferenceServiceSpec{
		Runtime: runtime,
		Model:   servingv1.ModelSpec{URI: uri},
		Settings: servingv1.SettingsSpec{
			Precision: "fp16",
			BatchSize: 4,
			GPUMemory: "24Gi",
		},
		Scaling: servingv1.ScalingSpec{Replicas: 2},
	}
}

func TestImageForRuntime(t *testing.T) {
	t.Setenv("RELATED_IMAGE_VLLM", "")
	t.Setenv("RELATED_IMAGE_SGLANG", "")

	if got := imageForRuntime("vllm"); got != defaultVLLMImage {
		t.Fatalf("vllm image = %q, want %q", got, defaultVLLMImage)
	}
	if got := imageForRuntime("sglang"); got != defaultSGLangImage {
		t.Fatalf("sglang image = %q, want %q", got, defaultSGLangImage)
	}
	if got := imageForRuntime("VLLM"); got != defaultVLLMImage {
		t.Fatalf("uppercase runtime should map to vllm image, got %q", got)
	}

	t.Setenv("RELATED_IMAGE_VLLM", "registry.example.com/custom-vllm:1.2.3")
	if got := imageForRuntime("vllm"); got != "registry.example.com/custom-vllm:1.2.3" {
		t.Fatalf("env override not applied, got %q", got)
	}
}

func TestCommandForRuntime(t *testing.T) {
	vllmCmd := strings.Join(commandForRuntime("vllm", spec("vllm", "meta-llama/Llama-3.1-8B-Instruct")), " ")
	if !strings.HasPrefix(vllmCmd, "vllm serve meta-llama/Llama-3.1-8B-Instruct") {
		t.Fatalf("vllm command should use vllm serve, got %q", vllmCmd)
	}
	if strings.Contains(vllmCmd, "sglang") {
		t.Fatalf("vllm command must not use sglang: %q", vllmCmd)
	}
	if !strings.Contains(vllmCmd, "--dtype float16") || !strings.Contains(vllmCmd, "--max-num-seqs 4") {
		t.Fatalf("vllm command missing mapped settings: %q", vllmCmd)
	}

	sglangCmd := strings.Join(commandForRuntime("sglang", spec("sglang", "Qwen/Qwen2.5-7B-Instruct")), " ")
	if !strings.Contains(sglangCmd, "sglang.launch_server") || !strings.Contains(sglangCmd, "--model-path Qwen/Qwen2.5-7B-Instruct") {
		t.Fatalf("sglang command wrong: %q", sglangCmd)
	}
	if !strings.Contains(sglangCmd, "--max-running-requests 4") {
		t.Fatalf("sglang command missing batch size: %q", sglangCmd)
	}
}

func TestMapPrecision(t *testing.T) {
	cases := map[string]string{"fp16": "float16", "fp32": "float32", "int8": "int8", "": "float16"}
	for in, want := range cases {
		if got := mapPrecision(in); got != want {
			t.Fatalf("mapPrecision(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestReplicasForDefaultsToOne(t *testing.T) {
	empty := &servingv1.LocalInferenceServiceSpec{}
	if got := replicasFor(empty); got != 1 {
		t.Fatalf("replicasFor(empty) = %d, want 1", got)
	}
	three := &servingv1.LocalInferenceServiceSpec{Scaling: servingv1.ScalingSpec{Replicas: 3}}
	if got := replicasFor(three); got != 3 {
		t.Fatalf("replicasFor(3) = %d, want 3", got)
	}
}

func TestMemoryLimitFor(t *testing.T) {
	withGPU := &servingv1.LocalInferenceServiceSpec{Settings: servingv1.SettingsSpec{GPUMemory: "24Gi"}}
	if got := memoryLimitFor(withGPU); got.String() != "24Gi" {
		t.Fatalf("memoryLimitFor(24Gi) = %s", got.String())
	}
	invalid := &servingv1.LocalInferenceServiceSpec{Settings: servingv1.SettingsSpec{GPUMemory: "not-a-quantity"}}
	if got := memoryLimitFor(invalid); got.String() != "8Gi" {
		t.Fatalf("memoryLimitFor(invalid) = %s, want 8Gi fallback", got.String())
	}
}

func TestResourcesForRequestsGPU(t *testing.T) {
	res := resourcesFor(&servingv1.LocalInferenceServiceSpec{})
	if _, ok := res.Requests[gpuResourceName]; !ok {
		t.Fatalf("GPU request missing from %v", res.Requests)
	}
	if _, ok := res.Limits[gpuResourceName]; !ok {
		t.Fatalf("GPU limit missing from %v", res.Limits)
	}
}

func TestHealthProbeTargetsHealthEndpoint(t *testing.T) {
	probe := healthProbe(10)
	if probe.HTTPGet == nil || probe.HTTPGet.Path != "/health" {
		t.Fatalf("probe should target /health, got %+v", probe.ProbeHandler)
	}
}

func TestImageOverrideOnlyWhenSet(t *testing.T) {
	os.Unsetenv("RELATED_IMAGE_VLLM")
	if got := imageForRuntime("vllm"); got != defaultVLLMImage {
		t.Fatalf("expected default image with no env, got %q", got)
	}
}

# local-inference-operator

![Go](https://img.shields.io/badge/go-%2300ADD8.svg?style=flat&logo=go&logoColor=white) ![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg) ![Kubernetes](https://img.shields.io/badge/kubernetes-compatible-blue?logo=kubernetes)

Kubernetes operator for local LLM inference. Declarative deployment via CRDs, GPU scheduling, node affinity, health checks.

## The problem

Deploying one LLM on Kubernetes is easy. Deploying ten, across different GPU types, with health checks and auto-restart, is where it gets messy. Each model needs a Deployment, a Service, GPU resource requests, node affinity for GPU nodes, and a health check that knows whether the model is actually serving, not just whether the pod is running.

## The idea

A `LocalInferenceService` CRD that abstracts all of that. You declare what model, what runtime (vLLM or SGLang), how many replicas. The operator creates and owns the Deployment and Service, wires up health probes, requests GPUs, and reports real readiness back on the resource.

## CRD

```yaml
apiVersion: serving.local-ome.com/v1
kind: LocalInferenceService
metadata:
  name: qwen-7b
spec:
  runtime: vllm
  model:
    uri: "Qwen/Qwen2.5-7B-Instruct"
    name: "Qwen 2.5 7B"
  settings:
    batchSize: 4
    precision: fp16
    maxTokens: 512
    gpuMemory: 24Gi
  scaling:
    replicas: 1
```

## Install

Requires a Kubernetes cluster with GPU nodes and `kubectl` pointed at it.

```bash
make install   # install the CRDs
make deploy    # deploy the controller manager
```

For local development:

```bash
make run       # run the controller against the current kubeconfig
```

Apply a sample:

```bash
kubectl apply -f config/samples/localinferenceservice.yaml
kubectl get localinferenceservices
```

## What it does

- **declarative deployment**: model deployments as CRDs. `kubectl get localinferenceservices` shows what is running.
- **runtime-specific wiring**: vLLM services start with `vllm serve`, SGLang services with `sglang.launch_server`, each with the right flags for that runtime.
- **GPU scheduling**: every inference container requests and limits `nvidia.com/gpu`. `settings.gpuMemory` maps onto the container memory limit, since Kubernetes schedules GPU count rather than GPU memory.
- **health checks**: readiness and liveness probes hit `/health`, so a pod that loaded a model but cannot serve yet stays out of the Service.
- **autoscaling**: set `scaling.autoScale: true` to create a CPU-targeted HPA. Turning it off deletes the HPA again.
- **garbage collection**: created Deployments and Services carry an owner reference, so deleting the CR removes them.
- **real status**: `status.phase`, `status.readyReplicas`, and an `Available` condition reflect the actual Deployment state.

## Configuration

| field | default | description |
|---|---|---|
| `runtime` | required | `vllm` or `sglang` |
| `model.uri` | required | model identifier passed to the runtime |
| `settings.precision` | `fp16` | `fp16`, `fp32`, or `int8` |
| `settings.batchSize` | `1` | mapped to `--max-num-seqs` (vLLM) or `--max-running-requests` (SGLang) |
| `settings.maxTokens` | `100` | request-time generation limit, recorded on the CRD only |
| `settings.gpuMemory` | `8Gi` | container memory limit in Kubernetes quantity form |
| `scaling.replicas` | `1` | number of pods |
| `scaling.autoScale` | `false` | create an HPA |
| `scaling.minReplicas` / `maxReplicas` | `1` / `10` | HPA bounds |
| `scaling.targetCPU` | `70` | HPA CPU target percentage |

Runtime images default to `vllm/vllm-openai:latest` and `lmsysorg/sglang:latest`, and can be pinned per environment with `RELATED_IMAGE_VLLM` and `RELATED_IMAGE_SGLANG`.

## Development

```bash
make test      # envtest suite + unit tests
make build     # build the manager binary
make manifests # regenerate CRDs and RBAC
make generate  # regenerate deepcopy code
```

## Related

- [inference-operator-tui](https://github.com/santura-dev/inference-operator-tui) - terminal UI for managing these CRDs

## License

MIT

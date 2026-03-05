# Local Inference Operator

Kubernetes operator for declarative deployment and management of local LLM inference services using vLLM and SGLang.

## Description

The Local Inference Operator simplifies LLM deployment on Kubernetes by providing a declarative API for model serving. It automates the creation of optimized inference pipelines with GPU resource management, multi-runtime support, and production monitoring.

## Features

- **Declarative API**: CRD-based model deployment specifications
- **Multi-Runtime Support**: vLLM and SGLang runtimes
- **GPU Resource Management**: Automatic GPU allocation and monitoring
- **Auto-Scaling**: Dynamic pod scaling based on load
- **Health Monitoring**: Built-in health checks and status reporting

## Getting Started

### Prerequisites
- Kubernetes cluster with GPU support
- NVIDIA GPU Operator installed
- kubectl configured
- Go 1.19+ (for development)

### Installation

**Install the CRDs:**
```bash
kubectl apply -f config/crd/
```

**Install RBAC:**
```bash
kubectl apply -f config/rbac/
```

**Deploy the operator:**
```bash
kubectl apply -f config/manager/
```

**Verify installation:**
```bash
kubectl get pods -n local-ome-system
```

### Usage

**Create a model service:**
```yaml
apiVersion: serving.local-ome.com/v1
kind: LocalInferenceService
metadata:
  name: phi-service
spec:
  runtime: sglang
  model: microsoft/phi-1_5
  settings:
    gpuMemoryUtilization: 0.8
    maxNumSeqs: 64
    maxModelLen: 2048
```

```bash
kubectl apply -f phi-service.yaml
```

**Check status:**
```bash
kubectl get localinferenceservices
kubectl describe localinferenceservice phi-service
```

**Access the service:**
```bash
kubectl get svc
curl http://service-endpoint/v1/completions \
  -H "Content-Type: application/json" \
  -d '{"model": "microsoft/phi-1_5", "prompt": "Hello"}'
```

### Supported Runtimes

**SGLang:**
- Optimized for long-context inference
- FlashAttention integration
- RadixAttention for memory efficiency

**vLLM:**
- PagedAttention for memory management
- Continuous batching
- Extensive model support

## Development

### Prerequisites
- Go 1.19+
- Kubebuilder 3.7+
- Docker

### Build and Test
```bash
make generate
make build
make test
make docker-build
```

### Local Development
```bash
make run
```

## Troubleshooting

**Pods not starting:**
```bash
kubectl describe pod <pod-name>
kubectl logs <pod-name>
```

**GPU allocation failures:**
```bash
kubectl get nodes -o yaml | grep nvidia
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Update documentation
6. Submit a pull request

## License

MIT License - see LICENSE file for details

## Related Projects

- [operator-tui](https://github.com/yourusername/operator-tui) - Management interface
- [vllm-tui](https://github.com/yourusername/vllm-tui) - Chat interface
- [kubectl-tui](https://github.com/yourusername/kubectl-tui) - kubectl interface


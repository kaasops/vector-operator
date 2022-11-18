### Added
- [[48](https://github.com/kaasops/vector-operator/pull/48)] **Features**: Add helm repo (with GitHub Pages)
- [[47](https://github.com/kaasops/vector-operator/pull/47)] **Features**: Init Helm chart

### v0.0.3
- [[46](https://github.com/kaasops/vector-operator/pull/46)] **Fix**: Fix error with envs forwarding from CR Vector

### v0.0.2
- [[45](https://github.com/kaasops/vector-operator/pull/45)] **Tests**: Add tests for k8s utils && refactor k8s utils
- [[44](https://github.com/kaasops/vector-operator/pull/44)] **Features**: Add validations errors for VectorPipeline
- [[37](https://github.com/kaasops/vector-operator/pull/37)] **Cleanup**: Fix context-in-struct warning
- [[32](https://github.com/kaasops/vector-operator/pull/32)] **Refactor**: Config build refactoring 
- [[40](https://github.com/kaasops/vector-operator/pull/40)] **Fix**: Sloved context forward errors


### v0.0.1
- Refactor: Refactor Pipeline for add ClusterVectorPipeline and checks
- Feature: Add field reason to CR Vector and VectorPipeline
- Feature: Add ConfigCheck for Vector
- Feature: Add ConfigCheck for VectorPipeline
- Feature: Add utils for reconciling Kubernetes resources
- Cleanup: Update Kustomize version in Makefile to v4.2.0 (for amd64 support)
- Agent: Init Vector Agent Controller

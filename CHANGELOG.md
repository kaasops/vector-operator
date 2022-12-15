### Added

### v0.0.10
- [[73]](https://github.com/kaasops/vector-operator/pull/73) **Fix** Add vector tolerations for configCheck pod

### v0.0.9
- [[72]](https://github.com/kaasops/vector-operator/pull/72) **Feature** Add vp to all category
- [[71]](https://github.com/kaasops/vector-operator/pull/71) **Fix** Fix panic on empty sinks and sources
- [[70]](https://github.com/kaasops/vector-operator/pull/70) **Fix** Fix nil vector panic
- [[65](https://github.com/kaasops/vector-operator/pull/69)] **Helm**: Add toleration-control for vector-operator deployment to helm chart
- [[65](https://github.com/kaasops/vector-operator/pull/69)] **Cleanup**: Add default value for podSecurityContext in chart values

### v0.0.8
- [[65](https://github.com/kaasops/vector-operator/pull/65)] **Refactor**: merge vp and cvp reconcile funcs
- [[64](https://github.com/kaasops/vector-operator/pull/64)] **Fix** Do not reconсile vector if vp check fail
- [[63](https://github.com/kaasops/vector-operator/pull/63)] **Fix** Fix configcheck gc

### v0.0.7
- [[61](https://github.com/kaasops/vector-operator/pull/61)] **Feature** Filter cache and disable time reconcile

### v0.0.6
- [[60](https://github.com/kaasops/vector-operator/pull/60)] **Fix**: Fix Vector agent DaemosSet for collect journald service logs
- [[60](https://github.com/kaasops/vector-operator/pull/60)] **Docs**: Add docs

### v0.0.5
- [[56](https://github.com/kaasops/vector-operator/pull/56)] **Fix**: Fix envs forwarding issue for configCheck 
- [[51](https://github.com/kaasops/vector-operator/pull/51)] **Fix**: Fix error with install vector CR in helm
- [[50](https://github.com/kaasops/vector-operator/pull/50)] **Fix**: Fix error with SA in helm && Update helm

### v0.0.4
- [[49](https://github.com/kaasops/vector-operator/pull/49)] **Refactor**: Add default resurces for configcheck
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

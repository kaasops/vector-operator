### v0.0.23
- [[105]](https://github.com/kaasops/vector-operator/pull/105) **Feature** Added config file gz compression for large configs

### v0.0.22

### v0.0.21
- [[104](https://github.com/kaasops/vector-operator/pull/104)] **Feature** Prepare helm for Openshift

### v0.0.20
- [[100](https://github.com/kaasops/vector-operator/pull/100)] **Fix** Create csv and fix rbac auto generation

### v0.0.19
- [[97](https://github.com/kaasops/vector-operator/pull/97)] **Fix** Check if ServiceMonitor CRD exists 

### v0.0.18
- [[95](https://github.com/kaasops/vector-operator/pull/95)] **Fix** Added ImagePullPolicy to Vector Agent

### v0.0.17
- [[93](https://github.com/kaasops/vector-operator/pull/93)] **Fix** Fix Condition type

### v0.0.16
- [[92](https://github.com/kaasops/vector-operator/pull/92)] **Fix** Added rbac for ServiceMonitros

### v0.0.15
- [[89](https://github.com/kaasops/vector-operator/pull/89)] **Feature** Added experemental config optimization option

### v0.0.14
- [[85](https://github.com/kaasops/vector-operator/pull/85)] **Feature** Add metrics exporter and ServiceMonitor creation

### v0.0.13
- [[83](https://github.com/kaasops/vector-operator/pull/83)] **Fix** Fix configcheck pods cleanup

### v0.0.12
- [[81](https://github.com/kaasops/vector-operator/pull/81)] **Feature** Add control for ConfigCheck params

### v0.0.11
- [[79](https://github.com/kaasops/vector-operator/pull/79)] **Fix** Remove vector.dev/exclude label 
- [[77](https://github.com/kaasops/vector-operator/pull/77)] **Feature** Concurrent pipeline checks

### v0.0.10
- [[73](https://github.com/kaasops/vector-operator/pull/73)] **Fix** Add vector tolerations for configCheck pod

### v0.0.9
- [[72](https://github.com/kaasops/vector-operator/pull/72)] **Feature** Add vp to all category
- [[71](https://github.com/kaasops/vector-operator/pull/71)] **Fix** Fix panic on empty sinks and sources
- [[70](https://github.com/kaasops/vector-operator/pull/70)] **Fix** Fix nil vector panic
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

# Security Policy

## Reporting

Report privately through [GitHub's vulnerability reporting form](https://github.com/kaasops/vector-operator/security/advisories/new).

Do not post vulnerability details in public issues, discussions or pull requests. If the form does not work for you, open an issue that asks only for a private security contact.

Please include:
- the affected component, operator and chart versions, and the Kubernetes and Vector versions;
- the permissions and network access an attacker needs;
- reproduction steps or a precise explanation based on the code, the manifests or Helm values involved, and the impact.

Remove credentials and personal data. Check AI-assisted findings yourself before you send them.

## Scope and trust model

In scope:
- the operator, the event collector and the checkpoint merger, and their release images;
- the Helm chart, CRDs and RBAC;
- the Vector configuration and workloads the operator generates, including config check pods and the config reloader.

The operator has cluster-wide permissions. `Vector`, `VectorAggregator`, `ClusterVectorPipeline`, `ClusterVectorAggregator` and the operator deployment are for trusted administrators.

Authors of namespaced `VectorPipeline` resources can be tenants. An agent pipeline collects logs only from its own namespace, and a pipeline can reference Secrets only in its own namespace. A way around these limits, access to another tenant's data or Secrets, privilege escalation, or a denial of service that affects other tenants is a vulnerability. Using permissions you already have, or configuring your own pipeline, is not.

Vulnerabilities in Vector itself go to the [Vector project](https://github.com/vectordotdev/vector/security). Dependency issues are in scope if they are exploitable through the operator or our images.

## Fixes and disclosure

Fixes go into the latest release. Backports are not guaranteed.

We are a small volunteer team and cannot promise response or fix deadlines. We agree on disclosure with you privately, and for a confirmed vulnerability we publish a GitHub Security Advisory and may request a CVE. Tell us if you have a disclosure deadline and how you want to be credited.

# Specification

- [Specification](#specification)
- [Vector Spec](#vector-spec)
  - [Api Spec](#api-spec)
- [VectorPipelineSpec (ClusterVectorPipelineSpec)](#vectorpipelinespec-clustervectorpipelinespec)



# Vector Spec
<table>
    <tr>
      <td rowspan="21">agent</td>
      <td>image</td>
      <td>Image for Vector agent. <code>timberio/vector:0.24.0-distroless-libc</code> by default</td>
    </tr>
    <tr>
        <td>dataDir</td>
        <td><a href="https://vector.dev/docs/reference/configuration/global-options/#data_dir">DataDir</a> for Vector Agent. `/vector-data-dir` by default</td>
    </tr>
    <tr>
        <td>expireMetricsSecs</td>
        <td><a href="https://vector.dev/docs/reference/configuration/global-options/#expire_metrics_secs">ExpireMetricsSecs</a> 300 by default</td>
    </tr>
    <tr>
        <td><a href="https://vector.dev/docs/reference/api/">api</a></td>
        <td><a href="https://github.com/kaasops/vector-operator/blob/main/docs/specification.md#api-spec">ApiSpec</a></td>
    </tr>
    <tr>
        <td>service</td>
        <td>Temporary field for enabling service for Vector DaemonSet. By default - <code>false</code></td>
    </tr>
    <tr>
        <td>imagePullSecrets</td>
        <td>ImagePullSecrets An optional list of references to secrets in the same namespace to use for pulling images from registries. By default not set</td>
    </tr>
    <tr>
        <td>resources</td>
        <td>Resources container resource request and limits, https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/. If not specified - default setting will be used</td>
    </tr>
    <tr>
        <td>affinity</td>
        <td>Affinity If specified, the pod's scheduling constraints. By default not set</td>
    </tr>
    <tr>
        <td>tolerations</td>
        <td>Tolerations for Vector DaemonSet. By default - <code>nil</code></td>
    </tr>
    <tr>
        <td>securityContext</td>
        <td>SecurityContext holds pod-level security attributes and common container settings. By default - not set</td>
    </tr>
    <tr>
        <td>containerSecurityContext</td>
        <td>securityContext holds security configuration that will be applied to a container.</td>
    </tr>
    <tr>
        <td>schedulerName</td>
        <td>SchedulerName - defines kubernetes scheduler name. By default - not set</td>
    </tr>
    <tr>
        <td>runtimeClassName</td>
        <td>RuntimeClassName - defines runtime class for kubernetes pod. By default - not set</td>
    </tr>
    <tr>
        <td>hostAliases</td>
        <td>HostAliases provides mapping between ip and hostnames, that would be propagated to pod.</td>
    </tr>
    <tr>
        <td>podSecurityPolicyName</td>
        <td>PodSecurityPolicyName - defines name for podSecurityPolicy in case of empty value, prefixedName will be used.</td>
    </tr>
    <tr>
        <td>readinessProbe</td>
        <td>Periodic probe of container service readiness. Container will be removed from service endpoints if the probe fails. By default - not set</td>
    </tr>
    <tr>
        <td>livenessProbe</td>
        <td>Periodic probe of container liveness. Container will be restarted if the probe fails. By default - not set</td>
    </tr>
    <tr>
        <td>volumes</td>
        <td>List of volumes that can be mounted by containers belonging to the pod.</td>
    </tr>
    <tr>
        <td>volumeMounts</td>
        <td>Pod volumes to mount into the container's filesystem.</td>
    </tr>
    <tr>
        <td>priorityClassName</td>
        <td>PriorityClassName assigned to the Pods.</td>
    </tr>
    <tr>
        <td>hostNetwork</td>
        <td>HostNetwork controls whether the pod may use the node network namespace.</td>
    </tr>
    <tr>
        <td>env</td>
        <td>Env that will be added to Vector pod. By default - not set</td>
    </tr>
</table>

## Api Spec
<table>
<tr>
      <td rowspan="5"><a href="https://vector.dev/docs/reference/api/">api</a></td>
    </tr>
    <tr>
        <td>address</td>
        <td>The network address to which the API should bind. If you’re running Vector in a Docker container, make sure to bind to <code>0.0.0.0</code>. Otherwise the API will not be exposed outside the container. By default - <code>0.0.0.0:8686</code></td>
    </tr>
    <tr>
        <td>enabled</td>
        <td>Whether the GraphQL API is enabled for this Vector instance. By default - <code>false</code></td>
    </tr>
    <tr>
        <td>playground</td>
        <td>Whether the GraphQL Playground is enabled for the API. The Playground is accessible via the /playground endpoint of the address set using the bind parameter. By default - <code>false</code></td>
    </tr>
    <tr>
        <td>healthcheck</td>
        <td>Enable ReadinessProbe and LivenessProbe via API <code>/health</code> endpoint. If probes enabled via VectorAgent, this setting will be ignored for that probe. By default - <code>false</code></td>
    </tr>
</table>


# VectorPipelineSpec (ClusterVectorPipelineSpec)
<table>
    <tr>
      <td>sources</td>
      <td>List of Sources</td>
    </tr>
    <tr>
      <td>transforms</td>
      <td>List of Transforms</td>
    </tr>
    <tr>
      <td>sinks</td>
      <td>List of Sinks</td>
    </tr>
</table>

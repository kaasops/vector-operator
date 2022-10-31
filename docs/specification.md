# Specification

- [Vector](#vector-spec)
- VectorPipeline
- ClusterVectorPipeline



# Vector Spec
<table>
    <tr>
      <td rowspan="9">agent</td>
      <td>image</td>
      <td>Image for Vector agent. <code>timberio/vector:0.24.0-distroless-libc</code> by default</td>
    </tr>
    <tr>
        <td>dataDir</td>
        <td><a href="https://vector.dev/docs/reference/configuration/global-options/#data_dir">DataDir</a> for Vector Agent. `/vector-data-dir` by default</td>
    </tr>
    <tr>
        <td rowspan="4"><a href="https://vector.dev/docs/reference/api/">api</a></td>
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
        <td>service</td>
        <td>Temporary field for enabling service for Vector DaemonSet. By default - <code>false</code></td>
    </tr>
    <tr>
        <td>tolerations</td>
        <td>Tolerations for Vector DaemonSet. By default - <code>nil</code></td>
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
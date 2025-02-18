# RayService troubleshooting

RayService is a Custom Resource Definition (CRD) designed for Ray Serve. In KubeRay, creating a RayService will first create a RayCluster and then
create Ray Serve applications once the RayCluster is ready. If the issue pertains to the data plane, specifically your Ray Serve scripts 
or Ray Serve configurations (`serveConfigV2`), troubleshooting may be challenging. This section provides some tips to help you debug these issues.

## Observability

### Method 1: Check KubeRay operator's logs for errors

```bash
kubectl logs $KUBERAY_OPERATOR_POD -n $YOUR_NAMESPACE | tee operator-log
```

The above command will redirect the operator's logs to a file called `operator-log`. You can then search for errors in the file.

### Method 2: Check RayService CR status

```bash
kubectl describe rayservice $RAYSERVICE_NAME -n $YOUR_NAMESPACE
```

You can check the status and events of the RayService CR to see if there are any errors.

### Method 3: Check logs of Ray Pods

You can also check the Ray Serve logs directly by accessing the log files on the pods. These log files contain system level logs from the Serve controller and HTTP proxy as well as access logs and user-level logs. See [Ray Serve Logging](https://docs.ray.io/en/latest/serve/production-guide/monitoring.html#ray-logging) and [Ray Logging](https://docs.ray.io/en/latest/ray-observability/user-guides/configure-logging.html#configure-logging) for more details.

```bash
kubectl exec -it $RAY_POD -n $YOUR_NAMESPACE -- bash
# Check the logs under /tmp/ray/session_latest/logs/serve/
```

### Method 4: Check Dashboard

```bash
kubectl port-forward $RAY_POD -n $YOUR_NAMESPACE --address 0.0.0.0 8265:8265
# Check $YOUR_IP:8265 in your browser
```

For more details about Ray Serve observability on the dashboard, you can refer to [the documentation](https://docs.ray.io/en/latest/ray-observability/getting-started.html#serve-view) and [the YouTube video](https://youtu.be/eqXfwM641a4).

### Method 5: Ray State CLI

You can use the [Ray State CLI](https://docs.ray.io/en/latest/ray-observability/reference/cli.html) on the head Pod to check the status of Ray Serve applications.

```bash
# Log into the head Pod
export HEAD_POD=$(kubectl get pods --selector=ray.io/node-type=head -o custom-columns=POD:metadata.name --no-headers)
kubectl exec -it $HEAD_POD -- ray summary actors

# [Example output]:
# ======== Actors Summary: 2023-07-11 17:58:24.625032 ========
# Stats:
# ------------------------------------
# total_actors: 14


# Table (group by class):
# ------------------------------------
#     CLASS_NAME                          STATE_COUNTS
# 0   ServeController                     ALIVE: 1
# 1   ServeReplica:fruit_app_OrangeStand  ALIVE: 1
# 2   HTTPProxyActor                      ALIVE: 3
# 3   ServeReplica:math_app_DAGDriver     ALIVE: 1
# 4   ServeReplica:math_app_Multiplier    ALIVE: 1
# 5   ServeReplica:math_app_create_order  ALIVE: 1
# 6   ServeReplica:fruit_app_DAGDriver    ALIVE: 1
# 7   ServeReplica:fruit_app_FruitMarket  ALIVE: 1
# 8   ServeReplica:math_app_Adder         ALIVE: 1
# 9   ServeReplica:math_app_Router        ALIVE: 1
# 10  ServeReplica:fruit_app_MangoStand   ALIVE: 1
# 11  ServeReplica:fruit_app_PearStand    ALIVE: 1
```

## Common issues

### Issue 1: Ray Serve script is incorrect.

We strongly recommend that you test your Ray Serve script locally or in a RayCluster before
deploying it to a RayService. Please refer to [rayserve-dev-doc.md](https://github.com/ray-project/kuberay/blob/master/docs/guidance/rayserve-dev-doc.md) for more details.

### Issue 2: `serveConfigV2` is incorrect.

For the sake of flexibility, we have set `serveConfigV2` as a YAML multi-line string in the RayService CR.
This implies that there is no strict type checking for the Ray Serve configurations in `serveConfigV2` field.
Some tips to help you debug the `serveConfigV2` field:

* Check [the documentation](https://docs.ray.io/en/latest/serve/api/#put-api-serve-applications) for the schema about
the Ray Serve Multi-application API `PUT "/api/serve/applications/"`.
* Unlike `serveConfig`, `serveConfigV2` adheres to the snake case naming convention. For example, `numReplicas` is used in `serveConfig`, while `num_replicas` is used in `serveConfigV2`. 

### Issue 3-1: The Ray image does not include the required dependencies.

You have two options to resolve this issue:

* Build your own Ray image with the required dependencies.
* Specify the required dependencies via `runtime_env` in `serveConfigV2` field.
  * For example, the MobileNet example requires `python-multipart`, which is not included in the Ray image `rayproject/ray-ml:2.5.0`.
Therefore, the YAML file includes `python-multipart` in the runtime environment. For more details, refer to [the MobileNet example](mobilenet-rayservice.md).

### Issue 3-2: Examples for troubleshooting dependency issues.

> Note: We highly recommend testing your Ray Serve script locally or in a RayCluster before deploying it to a RayService. This helps identify any dependency issues in the early stages. Please refer to [rayserve-dev-doc.md](https://github.com/ray-project/kuberay/blob/master/docs/guidance/rayserve-dev-doc.md) for more details.

In the [MobileNet example](mobilenet-rayservice.md), the [mobilenet.py](https://github.com/ray-project/serve_config_examples/blob/master/mobilenet/mobilenet.py) consists of two functions: `__init__()` and `__call__()`.
The function `__call__()` will only be called when the Serve application receives a request.

* Example 1: Remove `python-multipart` from the runtime environment in [the MobileNet YAML](../../ray-operator/config/samples/ray-service.mobilenet.yaml).
  * The `python-multipart` library is only required for the `__call__` method. Therefore, we can only observe the dependency issue when we send a request to the application.
  * Example error message:
    ```bash
    Unexpected error, traceback: ray::ServeReplica:mobilenet_ImageClassifier.handle_request() (pid=226, ip=10.244.0.9)
      .
      .
      .
      File "...", line 24, in __call__
        request = await http_request.form()
      File "/home/ray/anaconda3/lib/python3.7/site-packages/starlette/requests.py", line 256, in _get_form
        ), "The `python-multipart` library must be installed to use form parsing."
    AssertionError: The `python-multipart` library must be installed to use form parsing..
    ```

* Example 2: Update the image from `rayproject/ray-ml:2.5.0` to `rayproject/ray:2.5.0` in [the MobileNet YAML](../../ray-operator/config/samples/ray-service.mobilenet.yaml). The latter image does not include `tensorflow`.
  * The `tensorflow` library is imported in the [mobilenet.py](https://github.com/ray-project/serve_config_examples/blob/master/mobilenet/mobilenet.py).
  * Example error message:
    ```bash
    kubectl describe rayservices.ray.io rayservice-mobilenet

    # Example error message:
    Pending Service Status:
      Application Statuses:
        Mobilenet:
          ...
          Message:                  Deploying app 'mobilenet' failed:
            ray::deploy_serve_application() (pid=279, ip=10.244.0.12)
                ...
              File ".../mobilenet/mobilenet.py", line 4, in <module>
                from tensorflow.keras.preprocessing import image
            ModuleNotFoundError: No module named 'tensorflow'
    ```

### Issue 4: Incorrect `import_path`.

You can refer to [the documentation](https://docs.ray.io/en/latest/serve/api/doc/ray.serve.schema.ServeApplicationSchema.html#ray.serve.schema.ServeApplicationSchema.import_path) for more details about the format of `import_path`.
Taking [the MobileNet YAML file](../../ray-operator/config/samples/ray-service.mobilenet.yaml) as an example,
the `import_path` is `mobilenet.mobilenet:app`. The first `mobilenet` is the name of the directory in the `working_dir`,
the second `mobilenet` is the name of the Python file in the directory `mobilenet/`,
and `app` is the name of the variable representing Ray Serve application within the Python file.

```yaml
  serveConfigV2: |
    applications:
      - name: mobilenet
        import_path: mobilenet.mobilenet:app
        runtime_env:
          working_dir: "https://github.com/ray-project/serve_config_examples/archive/b393e77bbd6aba0881e3d94c05f968f05a387b96.zip"
          pip: ["python-multipart==0.0.6"]
```

### Issue 5: Fail to create / update Serve applications.

You may encounter the following error messages when KubeRay tries to create / update Serve applications:

#### Error message 1: `connect: connection refused`

```
Put "http://${HEAD_SVC_FQDN}:52365/api/serve/applications/": dial tcp $HEAD_IP:52365: connect: connection refused
```

For RayService, the KubeRay operator submits a request to the RayCluster for creating Serve applications once the head Pod is ready.
It's important to note that the Dashboard, Dashboard Agent and GCS may take a few seconds to start up after the head Pod is ready.
As a result, the request may fail a few times initially before the necessary components are fully operational.

If you continue to encounter this issue after waiting for 1 minute, it's possible that the dashboard or dashboard agent may have failed to start.
For more information, you can check the `dashboard.log` and `dashboard_agent.log` files located at `/tmp/ray/session_latest/logs/` on the head Pod.

#### Error message 2: `i/o timeout`

```
Put "http://${HEAD_SVC_FQDN}:52365/api/serve/applications/": dial tcp $HEAD_IP:52365: i/o timeout"
```

One possible cause of this issue could be a Kubernetes NetworkPolicy blocking the traffic between the Ray Pods and the dashboard agent's port (i.e., 52365).

### Issue 6: `runtime_env`

In `serveConfigV2`, you can specify the runtime environment for the Ray Serve applications via `runtime_env`.
Some common issues related to `runtime_env`:

* The `working_dir` points to a private AWS S3 bucket, but the Ray Pods do not have the necessary permissions to access the bucket.

* The NetworkPolicy blocks the traffic between the Ray Pods and the external URLs specified in `runtime_env`.

### Issue 7: Failed to get Serve application statuses.

You may encounter the following error message when KubeRay tries to get Serve application statuses:

```
Get "http://${HEAD_SVC_FQDN}:52365/api/serve/applications/": dial tcp $HEAD_IP:52365: connect: connection refused"
```

As mentioned in [Issue 5](#issue-5-fail-to-create--update-serve-applications), the KubeRay operator submits a `Put` request to the RayCluster for creating Serve applications once the head Pod is ready.
After the successful submission of the `Put` request to the dashboard agent, a `Get` request is sent to the dashboard agent port (i.e., 52365). 
The successful submission indicates that all the necessary components, including the dashboard agent, are fully operational. 
Therefore, unlike Issue 5, the failure of the `Get` request is not expected.

If you consistently encounter this issue, there are several possible causes:

* The dashboard agent process on the head Pod is not running. You can check the `dashboard_agent.log` file located at `/tmp/ray/session_latest/logs/` on the head Pod for more information. In addition, you can also perform an experiment to reproduce this cause by manually killing the dashboard agent process on the head Pod.
  ```bash
  # Step 1: Log in to the head Pod
  kubectl exec -it $HEAD_POD -n $YOUR_NAMESPACE -- bash

  # Step 2: Check the PID of the dashboard agent process
  ps aux
  # [Example output]
  # ray          156 ... 0:03 /.../python -u /.../ray/dashboard/agent.py --

  # Step 3: Kill the dashboard agent process
  kill 156

  # Step 4: Check the logs
  cat /tmp/ray/session_latest/logs/dashboard_agent.log

  # [Example output]
  # 2023-07-10 11:24:31,962 INFO web_log.py:206 -- 10.244.0.5 [10/Jul/2023:18:24:31 +0000] "GET /api/serve/applications/ HTTP/1.1" 200 13940 "-" "Go-http-client/1.1"
  # 2023-07-10 11:24:34,001 INFO web_log.py:206 -- 10.244.0.5 [10/Jul/2023:18:24:33 +0000] "GET /api/serve/applications/ HTTP/1.1" 200 13940 "-" "Go-http-client/1.1"
  # 2023-07-10 11:24:36,043 INFO web_log.py:206 -- 10.244.0.5 [10/Jul/2023:18:24:36 +0000] "GET /api/serve/applications/ HTTP/1.1" 200 13940 "-" "Go-http-client/1.1"
  # 2023-07-10 11:24:38,082 INFO web_log.py:206 -- 10.244.0.5 [10/Jul/2023:18:24:38 +0000] "GET /api/serve/applications/ HTTP/1.1" 200 13940 "-" "Go-http-client/1.1"
  # 2023-07-10 11:24:38,590 WARNING agent.py:531 -- Exiting with SIGTERM immediately...

  # Step 5: Open a new terminal and check the logs of the KubeRay operator
  kubectl logs $KUBERAY_OPERATOR_POD -n $YOUR_NAMESPACE | tee operator-log

  # [Example output]
  # Get \"http://rayservice-sample-raycluster-rqlsl-head-svc.default.svc.cluster.local:52365/api/serve/applications/\": dial tcp 10.96.7.154:52365: connect: connection refused
  ```

### Issue 8: A loop of restarting the RayCluster occurs when the Kubernetes cluster runs out of resources. (KubeRay v0.6.1 or earlier)

> Note: Currently, the KubeRay operator does not have a clear plan to handle situations where the Kubernetes cluster runs out of resources.
Therefore, we recommend ensuring that the Kubernetes cluster has sufficient resources to accommodate the serve application.

If the status of a serve application remains non-`RUNNING` for more than `serviceUnhealthySecondThreshold` seconds,
the KubeRay operator will consider the RayCluster as unhealthy and initiate the preparation of a new RayCluster.
A common cause of this issue is that the Kubernetes cluster does not have enough resources to accommodate the serve application.
In such cases, the KubeRay operator may continue to restart the RayCluster, leading to a loop of restarts.

We can also perform an experiment to reproduce this situation:

* A Kubernetes cluster with an 8-CPUs node
* [ray-service.insufficient-resources.yaml](https://gist.github.com/kevin85421/6a7779308aa45b197db8015aca0c1faf)
  * RayCluster:
    * The cluster has 1 head Pod with 4 physical CPUs, but `num-cpus` is set to 0 in `rayStartParams` to prevent any serve replicas from being scheduled on the head Pod.
    * The cluster also has 1 worker Pod with 1 CPU by default.
  * `serveConfigV2` specifies 5 serve deployments, each with 1 replica and a requirement of 1 CPU.

```bash
# Step 1: Get the number of CPUs available on the node
kubectl get nodes -o custom-columns=NODE:.metadata.name,ALLOCATABLE_CPU:.status.allocatable.cpu

# [Example output]
# NODE                 ALLOCATABLE_CPU
# kind-control-plane   8

# Step 2: Install a KubeRay operator.

# Step 3: Create a RayService with autoscaling enabled.
kubectl apply -f ray-service.insufficient-resources.yaml

# Step 4: The Kubernetes cluster will not have enough resources to accommodate the serve application.
kubectl describe rayservices.ray.io rayservice-sample -n $YOUR_NAMESPACE

# [Example output]
# fruit_app_DAGDriver:
#   Health Last Update Time:  2023-07-11T02:10:02Z
#   Last Update Time:         2023-07-11T02:10:35Z
#   Message:                  Deployment "fruit_app_DAGDriver" has 1 replicas that have taken more than 30s to be scheduled. This may be caused by waiting for the cluster to auto-scale, or waiting for a runtime environment to install. Resources required for each replica: {"CPU": 1.0}, resources available: {}.
#   Status:                   UPDATING

# Step 5: A new RayCluster will be created after `serviceUnhealthySecondThreshold` (300s here) seconds.
# Check the logs of the KubeRay operator to find the reason for restarting the RayCluster.
kubectl logs $KUBERAY_OPERATOR_POD -n $YOUR_NAMESPACE | tee operator-log

# [Example output]
# 2023-07-11T02:14:58.109Z	INFO	controllers.RayService	Restart RayCluster	{"appName": "fruit_app", "restart reason": "The status of the serve application fruit_app has not been RUNNING for more than 300.000000 seconds. Hence, KubeRay operator labels the RayCluster unhealthy and will prepare a new RayCluster."}
# 2023-07-11T02:14:58.109Z	INFO	controllers.RayService	Restart RayCluster	{"deploymentName": "fruit_app_FruitMarket", "appName": "fruit_app", "restart reason": "The status of the serve deployment fruit_app_FruitMarket or the serve application fruit_app has not been HEALTHY/RUNNING for more than 300.000000 seconds. Hence, KubeRay operator labels the RayCluster unhealthy and will prepare a new RayCluster. The message of the serve deployment is: Deployment \"fruit_app_FruitMarket\" has 1 replicas that have taken more than 30s to be scheduled. This may be caused by waiting for the cluster to auto-scale, or waiting for a runtime environment to install. Resources required for each replica: {\"CPU\": 1.0}, resources available: {}."}
# .
# .
# .
# 2023-07-11T02:14:58.122Z	INFO	controllers.RayService	Restart RayCluster	{"ServiceName": "default/rayservice-sample", "AvailableWorkerReplicas": 1, "DesiredWorkerReplicas": 5, "restart reason": "The serve application is unhealthy, restarting the cluster. If the AvailableWorkerReplicas is not equal to DesiredWorkerReplicas, this may imply that the Autoscaler does not have enough resources to scale up the cluster. Hence, the serve application does not have enough resources to run. Please check https://github.com/ray-project/kuberay/blob/master/docs/guidance/rayservice-troubleshooting.md for more details.", "RayCluster": {"apiVersion": "ray.io/v1alpha1", "kind": "RayCluster", "namespace": "default", "name": "rayservice-sample-raycluster-hvd9f"}}
```
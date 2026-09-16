package environments

import (
	"fmt"
	"strconv"
)

type object = map[string]any

func labels(token string, e *Environment) object {
	return object{"app.kubernetes.io/managed-by": "aycorn", "aycorn.dev/installation": token, "aycorn.dev/environment": strconv.Itoa(e.ID)}
}
func namespace(token string, e *Environment) string { return fmt.Sprintf("aycorn-%s-e%d", token, e.ID) }
func resource(api, kind, name, ns string, metadataLabels object, spec object) object {
	metadata := object{"name": name, "labels": metadataLabels}
	if ns != "" {
		metadata["namespace"] = ns
	}
	result := object{"apiVersion": api, "kind": kind, "metadata": metadata}
	if spec != nil {
		result["spec"] = spec
	}
	return result
}
func podSecurity() object {
	return object{"runAsNonRoot": true, "runAsUser": 1000, "runAsGroup": 1000, "fsGroup": 1000, "seccompProfile": object{"type": "RuntimeDefault"}}
}
func containerSecurity(readOnly bool) object {
	return object{"allowPrivilegeEscalation": false, "readOnlyRootFilesystem": readOnly, "capabilities": object{"drop": []string{"ALL"}}}
}
func resources(cpu, memory int, ephemeral string) object {
	return object{"requests": object{"cpu": "100m", "memory": "128Mi", "ephemeral-storage": "128Mi"}, "limits": object{"cpu": fmt.Sprintf("%dm", cpu), "memory": fmt.Sprintf("%dMi", memory), "ephemeral-storage": ephemeral}}
}

func foundation(token string, e *Environment) []object {
	ns := namespace(token, e)
	ls := labels(token, e)
	namespaceLabels := labels(token, e)
	namespaceLabels["pod-security.kubernetes.io/enforce"] = "restricted"
	namespaceLabels["pod-security.kubernetes.io/enforce-version"] = "v1.35"
	claim := object{"accessModes": []string{"ReadWriteOnce"}, "resources": object{"requests": object{"storage": fmt.Sprintf("%dGi", e.Settings.StorageGiB)}}}
	if e.Settings.StorageClass != "" {
		claim["storageClassName"] = e.Settings.StorageClass
	}
	return []object{
		resource("v1", "Namespace", ns, "", namespaceLabels, nil),
		resource("v1", "ResourceQuota", "budget", ns, ls, object{"hard": object{"pods": "4", "persistentvolumeclaims": "1", "services": "1", "secrets": "2", "count/deployments.apps": "1", "count/jobs.batch": "1", "requests.storage": fmt.Sprintf("%dGi", e.Settings.StorageGiB), "limits.cpu": fmt.Sprintf("%dm", e.Settings.CPU+e.Settings.TestCPU), "limits.memory": fmt.Sprintf("%dMi", e.Settings.Memory+e.Settings.TestMemory), "limits.ephemeral-storage": "3Gi"}}),
		resource("networking.k8s.io/v1", "NetworkPolicy", "isolate", ns, ls, object{"podSelector": object{}, "policyTypes": []string{"Ingress", "Egress"}, "ingress": []any{}, "egress": []any{}}),
		resource("v1", "PersistentVolumeClaim", "data", ns, ls, claim),
	}
}
func deployment(token, mainURL string, e *Environment) object {
	ns := namespace(token, e)
	ls := labels(token, e)
	selector := object{"aycorn.dev/environment": strconv.Itoa(e.ID), "aycorn.dev/component": "preview"}
	podLabels := labels(token, e)
	podLabels["aycorn.dev/component"] = "preview"
	env := []object{}
	for _, entry := range [][2]string{{"AYCORN_PREVIEW", "1"}, {"AYCORN_HOST", "0.0.0.0"}, {"AYCORN_PORT", strconv.Itoa(e.Settings.Port)}, {"AYCORN_DB", "/data/app.db"}, {"AYCORN_PREVIEW_BRANCH", e.Branch}, {"AYCORN_PREVIEW_REVISION", e.Commit}, {"AYCORN_PREVIEW_DIGEST", e.Digest}, {"AYCORN_MAIN_URL", mainURL}, {"PORT", strconv.Itoa(e.Settings.Port)}, {"HOME", "/tmp"}} {
		env = append(env, object{"name": entry[0], "value": entry[1]})
	}
	container := object{"name": "app", "image": e.Image, "imagePullPolicy": "IfNotPresent", "securityContext": containerSecurity(true), "env": env, "ports": []object{{"name": "http", "containerPort": e.Settings.Port}}, "resources": resources(e.Settings.CPU, e.Settings.Memory, "512Mi"),
		"volumeMounts":   []object{{"name": "data", "mountPath": "/data"}, {"name": "scratch", "mountPath": "/tmp"}},
		"readinessProbe": object{"httpGet": object{"path": e.Settings.HealthPath, "port": "http"}, "periodSeconds": 3, "timeoutSeconds": 2, "failureThreshold": 3},
		"startupProbe":   object{"httpGet": object{"path": e.Settings.HealthPath, "port": "http"}, "periodSeconds": 3, "timeoutSeconds": 2, "failureThreshold": 80},
		"livenessProbe":  object{"httpGet": object{"path": e.Settings.HealthPath, "port": "http"}, "periodSeconds": 15, "timeoutSeconds": 2, "failureThreshold": 3}}
	if len(e.Settings.Command) > 0 {
		container["command"] = e.Settings.Command
	}
	pod := object{"automountServiceAccountToken": false, "securityContext": podSecurity(), "terminationGracePeriodSeconds": 20, "containers": []object{container}, "volumes": []object{{"name": "data", "persistentVolumeClaim": object{"claimName": "data"}}, {"name": "scratch", "emptyDir": object{"sizeLimit": "256Mi"}}}}
	if e.Settings.PullSecretName != "" {
		pod["imagePullSecrets"] = []object{{"name": "registry"}}
	}
	return resource("apps/v1", "Deployment", "app", ns, ls, object{"replicas": 1, "strategy": object{"type": "Recreate"}, "selector": object{"matchLabels": selector}, "template": object{"metadata": object{"labels": podLabels}, "spec": pod}})
}
func serviceManifest(token string, e *Environment) object {
	return resource("v1", "Service", "app", namespace(token, e), labels(token, e), object{"type": "ClusterIP", "selector": object{"aycorn.dev/environment": strconv.Itoa(e.ID), "aycorn.dev/component": "preview"}, "ports": []object{{"name": "http", "port": e.Settings.Port, "targetPort": "http"}}})
}

func testJob(token string, e *Environment) object {
	ls := labels(token, e)
	ls["aycorn.dev/component"] = "test"
	pod := object{"automountServiceAccountToken": false, "restartPolicy": "Never", "securityContext": podSecurity(), "terminationGracePeriodSeconds": 5,
		"containers": []object{{"name": "tests", "image": e.TestImage, "imagePullPolicy": "IfNotPresent", "command": e.Settings.TestCommand, "securityContext": containerSecurity(false), "resources": resources(e.Settings.TestCPU, e.Settings.TestMemory, "2Gi"), "env": []object{{"name": "HOME", "value": "/tmp"}, {"name": "TMPDIR", "value": "/tmp"}, {"name": "GOCACHE", "value": "/tmp/go-build"}, {"name": "npm_config_cache", "value": "/tmp/npm"}}, "volumeMounts": []object{{"name": "scratch", "mountPath": "/tmp"}}}}, "volumes": []object{{"name": "scratch", "emptyDir": object{"sizeLimit": "1Gi"}}}}
	if e.Settings.PullSecretName != "" {
		pod["imagePullSecrets"] = []object{{"name": "registry"}}
	}
	return resource("batch/v1", "Job", "tests", namespace(token, e), labels(token, e), object{"backoffLimit": 0, "activeDeadlineSeconds": e.Settings.TimeoutSeconds, "template": object{"metadata": object{"labels": ls}, "spec": pod}})
}
func testNetwork(token string, e *Environment) object {
	return resource("networking.k8s.io/v1", "NetworkPolicy", "test-internet", namespace(token, e), labels(token, e), object{"podSelector": object{"matchLabels": object{"aycorn.dev/component": "test"}}, "policyTypes": []string{"Egress"}, "egress": []object{
		{"to": []object{{"namespaceSelector": object{"matchLabels": object{"kubernetes.io/metadata.name": "kube-system"}}}}, "ports": []object{{"protocol": "UDP", "port": 53}, {"protocol": "TCP", "port": 53}}},
		{"to": []object{{"ipBlock": object{"cidr": "0.0.0.0/0", "except": []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16", "127.0.0.0/8", "169.254.0.0/16"}}}}, "ports": []object{{"protocol": "TCP", "port": 443}, {"protocol": "TCP", "port": 80}}},
	}})
}

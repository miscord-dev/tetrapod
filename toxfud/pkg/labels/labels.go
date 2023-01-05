package labels

import "k8s.io/apimachinery/pkg/types"

func ForNode(clusterName, nodeName string) map[string]string {
	return map[string]string{
		"client.miscord.win/cluster": clusterName,
		"client.miscord.win/node":    nodeName,
	}
}

func NodeTypeForNode(clusterName, nodeName string) map[string]string {
	labels := ForNode(clusterName, nodeName)

	labels["client.miscord.win/type"] = "node"

	return labels
}

func NodeTypeForPodCIDR(clusterName, nodeName string) map[string]string {
	labels := ForNode(clusterName, nodeName)

	labels["client.miscord.win/type"] = "pod-cidr"

	return labels
}

func NodeTypeForExtraPodCIDRAll(clusterName, nodeName string) map[string]string {
	labels := ForNode(clusterName, nodeName)

	labels["client.miscord.win/type"] = "extra-pod-cidr"

	return labels
}

const (
	podNamespaceKey = "client.miscord.win/namespace"
	podNameKey      = "client.miscord.win/name"
)

func NodeTypeForExtraPodCIDR(clusterName, nodeName, namespace, name string) map[string]string {
	labels := NodeTypeForExtraPodCIDRAll(clusterName, nodeName)

	labels[podNamespaceKey] = namespace
	labels[podNameKey] = name

	return labels
}

func NamespacedNameFromExtraPodCIDR(labels map[string]string) types.NamespacedName {
	return types.NamespacedName{
		Namespace: labels[podNamespaceKey],
		Name:      labels[podNameKey],
	}
}

const AnnotationExtraPodCIDRTemplateKey = "toxfu.miscord.win/extra-template"

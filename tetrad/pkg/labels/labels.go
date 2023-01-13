package labels

import (
	"strings"

	"github.com/miscord-dev/tetrapod/tetrad/pkg/util"
	"k8s.io/apimachinery/pkg/types"
)

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

func PodCIDRTypeForNode(clusterName, nodeName, templateName string) map[string]string {
	labels := ForNode(clusterName, nodeName)

	labels["client.miscord.win/type"] = "pod-cidr"
	if templateName != "" {
		labels[TemplateNameLabelKey] = templateName
	}

	return labels
}

func ExtraPodCIDRTypeForNodeAll(clusterName, nodeName, templateName string) map[string]string {
	labels := ForNode(clusterName, nodeName)

	labels["client.miscord.win/type"] = "extra-pod-cidr"
	if templateName != "" {
		labels[TemplateNameLabelKey] = templateName
	}

	return labels
}

const (
	podNamespaceKey = "client.miscord.win/namespace"
	podNameKey      = "client.miscord.win/name"
)

func ExtraPodCIDRTypeForNode(clusterName, nodeName, namespace, name, templateName string) map[string]string {
	labels := ExtraPodCIDRTypeForNodeAll(clusterName, nodeName, templateName)

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

const AnnotationExtraPodCIDRTemplatesKey = "tetrapod.miscord.win/extra-templates"

func ExtraPODCIDRTemplateNames(annotationValue string) []string {
	extraTemplates := strings.Split(annotationValue, ",")
	for i := range extraTemplates {
		extraTemplates[i] = strings.TrimSpace(extraTemplates[i])
	}
	extraTemplates = util.Uniq(extraTemplates)

	return extraTemplates
}

const TemplateNameLabelKey = "client.miscord.win/template-name"

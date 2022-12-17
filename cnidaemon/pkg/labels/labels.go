package labels

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

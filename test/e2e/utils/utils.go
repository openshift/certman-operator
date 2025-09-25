package utils

import (
	"os/exec"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/log"
)

var logger = log.Log

func IsPodRunning(namespace string) bool {
	cmd := exec.Command("oc", "get", "pods", "-n", namespace, "-o", "jsonpath={range .items[*]}{.metadata.name}:{.status.phase}{\"\\n\"}{end}")
	output, err := cmd.Output()
	if err != nil {
		logger.Error(err, "Failed to run 'oc get pods'")
		return false
	}

	lines := strings.Split(string(output), "\n")
	logger.Info("Printing lines", lines)
	for _, line := range lines {
		if strings.HasPrefix(line, "certman-operator") && strings.Contains(line, "Running") {
			return true
		}
	}
	return false
}

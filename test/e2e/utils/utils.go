package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	syaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes"
)

type CertConfig struct {
	ClusterName    string
	BaseDomain     string
	TestNamespace  string
	CertSecretName string
	OCMClusterID   string
}

func SetupHiveCRDs(ctx context.Context, apiExtClient apiextensionsclient.Interface) error {
	const crdURL = "https://raw.githubusercontent.com/openshift/hive/master/config/crds/hive.openshift.io_clusterdeployments.yaml"
	const crdName = "clusterdeployments.hive.openshift.io"

	log.Printf("Downloading Hive CRD from: %s", crdURL)
	resp, err := http.Get(crdURL)
	if err != nil {
		return fmt.Errorf("failed to download Hive CRD: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download Hive CRD: HTTP status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read Hive CRD body: %w", err)
	}

	decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)
	var newCRD apiextensionsv1.CustomResourceDefinition
	if err := decoder.Decode(&newCRD); err != nil {
		return fmt.Errorf("failed to decode Hive CRD YAML: %w", err)
	}

	_, err = apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, &newCRD, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		log.Println("Hive CRD already exists. Attempting update.")

		// Fetching existing CRD to get the resourceVersion
		existingCRD, getErr := apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
		if getErr != nil {
			return fmt.Errorf("failed to get existing Hive CRD: %w", getErr)
		}

		// Copying the resourceVersion to the new CRD so the update is valid
		newCRD.ResourceVersion = existingCRD.ResourceVersion

		_, err = apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Update(ctx, &newCRD, metav1.UpdateOptions{})
	}

	if err != nil {
		return fmt.Errorf("failed to create or update Hive CRD: %w", err)
	}

	log.Println("Hive CRD applied successfully.")
	return nil
}

func SetupCertman(ctx context.Context, kubeClient kubernetes.Interface, apiExtClient apiextensionsclient.Interface) error {
	const (
		namespace     = "certman-operator"
		configMapName = "certman-operator"
		crdURL        = "https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/crds/certman.managed.openshift.io_certificaterequests.yaml"
	)
	crdName := "certificaterequests.certman.managed.openshift.io"

	_, err := kubeClient.CoreV1().Namespaces().Get(ctx, namespace, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		log.Printf("Namespace '%s' not found. Creating namespace", namespace)
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: namespace,
			},
		}

		if _, err := kubeClient.CoreV1().Namespaces().Create(ctx, ns, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create namespace '%s': %w", namespace, err)
		}

		log.Printf("Namespace '%s' created.", namespace)

	} else if err != nil {
		return fmt.Errorf("error getting namespace '%s': %w", namespace, err)
	} else {
		log.Printf("Namespace '%s' already exists.", namespace)
	}

	_, err = apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		log.Printf("CRD '%s' not found. Downloading and applying from: %s", crdName, crdURL)

		resp, err := http.Get(crdURL)
		if err != nil {
			return fmt.Errorf("failed to download CRD from %s: %w", crdURL, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to download CRD: HTTP status %d", resp.StatusCode)
		}

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read CRD body: %w", err)
		}

		decoder := syaml.NewDecodingSerializer(unstructured.UnstructuredJSONScheme)
		crd := &apiextensionsv1.CustomResourceDefinition{}
		_, _, err = decoder.Decode(data, nil, crd)

		if err != nil {
			return fmt.Errorf("failed to decode CRD YAML: %w", err)
		}

		if _, err := apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Create(ctx, crd, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create CRD '%s': %w", crdName, err)
		}

		log.Printf("CRD '%s' applied.", crdName)

	} else if err != nil {
		return fmt.Errorf("error getting CRD '%s': %w", crdName, err)
	} else {
		log.Printf("CRD '%s' already exists.", crdName)
	}

	_, err = kubeClient.CoreV1().ConfigMaps(namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		log.Printf("ConfigMap '%s' not found in namespace '%s' Creating ConfigMap", configMapName, namespace)

		cm := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: namespace,
			},
			Data: map[string]string{
				"default_notification_email_address": "teste2e@redhat.com",
			},
		}

		if _, err := kubeClient.CoreV1().ConfigMaps(namespace).Create(ctx, cm, metav1.CreateOptions{}); err != nil {
			return fmt.Errorf("failed to create ConfigMap '%s': %w", configMapName, err)
		}

		log.Printf("ConfigMap '%s' created in namespace '%s'.", configMapName, namespace)

	} else if err != nil {
		return fmt.Errorf("error getting ConfigMap '%s': %w", configMapName, err)
	} else {
		log.Printf("ConfigMap '%s' already exists in namespace '%s'.", configMapName, namespace)
	}

	log.Println("Certman setup completed successfully.")
	return nil
}

func SetupAWSCreds(ctx context.Context, kubeClient kubernetes.Interface) error {
	const (
		namespace  = "certman-operator"
		secretName = "aws"
	)

	awsAccessKey, awsSecretKey := getSecretAndAccessKeys()
	if awsAccessKey == "" || awsSecretKey == "" {
		return fmt.Errorf("AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY must be set")
	}

	_, err := kubeClient.CoreV1().Secrets(namespace).Get(ctx, secretName, metav1.GetOptions{})

	if err == nil {
		log.Printf("AWS Secret '%s' already exists in namespace '%s'. Skipping creation.", secretName, namespace)
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("error checking for existing AWS secret: %w", err)
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: namespace,
		},
		StringData: map[string]string{
			"aws_access_key_id":     awsAccessKey,
			"aws_secret_access_key": awsSecretKey,
		},
		Type: corev1.SecretTypeOpaque,
	}

	_, err = kubeClient.CoreV1().Secrets(namespace).Create(ctx, secret, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("failed to create AWS secret: %w", err)
	}

	log.Printf("AWS Secret '%s' created successfully in namespace '%s'.", secretName, namespace)
	return nil
}

func GetEnvOrDefault(envVar, defaultValue string) string {
	if value := os.Getenv(envVar); value != "" {
		return value
	}
	return defaultValue
}

func getSecretAndAccessKeys() (accesskey, secretkey string) {

	accesskey = SanitizeInput(GetEnvOrDefault("AWS_ACCESS_KEY", "testAccessKey"))
	secretkey = SanitizeInput(GetEnvOrDefault("AWS_SECRET_ACCESS_KEY", "testSecretAccessKey"))
	return

}

// G204 lint issue for exec.command
func SanitizeInput(input string) string {
	return "\"" + strings.ReplaceAll(input, "\"", "\\\"") + "\""
}

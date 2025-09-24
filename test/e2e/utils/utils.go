package utils

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"

	"os"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	syaml "k8s.io/apimachinery/pkg/runtime/serializer/yaml"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
)

func DownloadAndApplyCRD(ctx context.Context, apiExtClient apiextensionsclient.Interface, crdURL, crdName string) error {
	log.Printf("CRD '%s' not found. Downloading and applying from: %s", crdName, crdURL)

	// Validate URL
	parsedURL, err := url.ParseRequestURI(crdURL)
	if err != nil {
		return fmt.Errorf("invalid CRD URL: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create HTTP request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download CRD from %s: %w", crdURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to download CRD. HTTP status %d", resp.StatusCode)
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
	return nil
}

func ApplyManifestsFromURLs(ctx context.Context, cfg *rest.Config, manifestURLs []string) error {
	// Create discovery client and dynamic client from REST config
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create discovery client: %w", err)
	}

	gr, err := restmapper.GetAPIGroupResources(dc)
	if err != nil {
		return fmt.Errorf("failed to get API group resources: %w", err)
	}
	mapper := restmapper.NewDiscoveryRESTMapper(gr)

	dynamicClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return fmt.Errorf("failed to create dynamic client: %w", err)
	}

	for _, manifestURL := range manifestURLs {
		log.Printf("Downloading manifest from: %s", manifestURL)

		parsedURL, err := url.ParseRequestURI(manifestURL)
		if err != nil {
			return fmt.Errorf("invalid manifest URL: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
		if err != nil {
			return fmt.Errorf("failed to create HTTP request: %w", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("failed to download manifest from %s: %w", manifestURL, err)
		}
		defer resp.Body.Close()

		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read manifest from %s: %w", manifestURL, err)
		}

		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)

		for {
			var rawObj map[string]interface{}
			if err := decoder.Decode(&rawObj); err != nil {
				if err == io.EOF {
					break
				}
				return fmt.Errorf("failed to decode YAML from %s: %w", manifestURL, err)
			}
			if len(rawObj) == 0 {
				continue
			}

			obj := &unstructured.Unstructured{Object: rawObj}
			gvk := obj.GroupVersionKind()

			mapping, err := mapper.RESTMapping(gvk.GroupKind(), gvk.Version)
			if err != nil {
				return fmt.Errorf("failed to get REST mapping for GVK %v: %w", gvk, err)
			}

			var dri dynamic.ResourceInterface
			if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
				ns := obj.GetNamespace()
				if ns == "" {
					ns = "certman-operator"
					obj.SetNamespace(ns)
				}
				dri = dynamicClient.Resource(mapping.Resource).Namespace(ns)
			} else {
				dri = dynamicClient.Resource(mapping.Resource)
			}

			_, err = dri.Create(ctx, obj, metav1.CreateOptions{})
			if apierrors.IsAlreadyExists(err) {
				log.Printf("Resource %s/%s already exists, skipping.", obj.GetNamespace(), obj.GetName())
				continue
			}
			if err != nil {
				return fmt.Errorf("failed to create resource %s/%s: %w", obj.GetNamespace(), obj.GetName(), err)
			}
			log.Printf("Successfully applied resource: %s/%s", obj.GetNamespace(), obj.GetName())
		}
	}

	return nil
}

func SetupHiveCRDs(ctx context.Context, apiExtClient apiextensionsclient.Interface) error {
	const crdURL = "https://raw.githubusercontent.com/openshift/hive/master/config/crds/hive.openshift.io_clusterdeployments.yaml"
	const crdName = "clusterdeployments.hive.openshift.io"

	_, err := apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return DownloadAndApplyCRD(ctx, apiExtClient, crdURL, crdName)
	} else if err != nil {
		return fmt.Errorf("error getting CRD '%s': %w", crdName, err)
	}

	log.Printf("CRD '%s' already exists.", crdName)
	return nil
}

// SetupCertman ensures namespace, CRD, ConfigMap and applies operator manifests
func SetupCertman(ctx context.Context, kubeClient kubernetes.Interface, apiExtClient apiextensionsclient.Interface, cfg *rest.Config) error {
	const (
		namespace     = "certman-operator"
		configMapName = "certman-operator"
		crdURL        = "https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/crds/certman.managed.openshift.io_certificaterequests.yaml"
		crdName       = "certificaterequests.certman.managed.openshift.io"
	)

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

	// Checking CRD exists or create it
	_, err = apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Get(ctx, crdName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		if err := DownloadAndApplyCRD(ctx, apiExtClient, crdURL, crdName); err != nil {
			return err
		}
	} else if err != nil {
		return fmt.Errorf("error getting CRD '%s': %w", crdName, err)
	} else {
		log.Printf("CRD '%s' already exists.", crdName)
	}

	// Ensuring ConfigMap exists or create it
	_, err = kubeClient.CoreV1().ConfigMaps(namespace).Get(ctx, configMapName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		log.Printf("ConfigMap '%s' not found in namespace '%s'. Creating ConfigMap", configMapName, namespace)

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

	manifestURLs := []string{
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/service_account.yaml",
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/role.yaml",
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/role_binding.yaml",
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/operator.yaml",
	}

	if err := ApplyManifestsFromURLs(ctx, cfg, manifestURLs); err != nil {
		return fmt.Errorf("failed to apply certman operator manifests: %w", err)
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

func CleanupHive(ctx context.Context, apiExtClient apiextensionsclient.Interface) error {
	const hiveCRDName = "clusterdeployments.hive.openshift.io"

	err := apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, hiveCRDName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		log.Printf("Hive CRD not found; nothing to delete")
		return nil
	} else if err != nil {
		return fmt.Errorf("error deleting Hive CRD: %w", err)
	}

	log.Printf("Hive CRD deleted successfully")
	return nil
}

func CleanupCertman(ctx context.Context, kubeClient kubernetes.Interface, apiExtClient apiextensionsclient.Interface, dynamicClient dynamic.Interface, mapper meta.RESTMapper) error {
	const (
		certmanCRDName = "certificaterequests.certman.managed.openshift.io"
		operatorNS     = "certman-operator"
	)

	err := apiExtClient.ApiextensionsV1().CustomResourceDefinitions().Delete(ctx, certmanCRDName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		log.Printf("Certman CRD not found nothing to delete")
	} else if err != nil {
		return fmt.Errorf("error deleting Certman CRD: %w", err)
	} else {
		log.Printf("Certman CRD deleted successfully")
	}

	err = kubeClient.CoreV1().Namespaces().Delete(ctx, operatorNS, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		log.Printf("Namespace %s not found nothing to delete", operatorNS)
	} else if err != nil {
		return fmt.Errorf("error deleting namespace %s: %w", operatorNS, err)
	} else {
		log.Printf("Namespace %s deleted successfully", operatorNS)
	}

	manifestURLs := []string{
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/service_account.yaml",
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/role.yaml",
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/role_binding.yaml",
		"https://raw.githubusercontent.com/openshift/certman-operator/master/deploy/operator.yaml",
	}

	for _, manifestURL := range manifestURLs {
		log.Printf("Downloading manifest for cleanup: %s", manifestURL)

		parsedURL, err := url.ParseRequestURI(manifestURL)
		if err != nil {
			log.Printf("Invalid manifest URL %s: %v", manifestURL, err)
			continue
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, parsedURL.String(), nil)
		if err != nil {
			log.Printf("Failed to create HTTP request for %s: %v", manifestURL, err)
			continue
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Printf("Failed to download manifest %s: %v", manifestURL, err)
			continue
		}

		data, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Failed to read manifest %s: %v", manifestURL, err)
			continue
		}

		decoder := yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)

		for {
			var rawObj map[string]interface{}
			if err := decoder.Decode(&rawObj); err != nil {
				if err == io.EOF {
					break
				}
				log.Printf("Failed to decode YAML %s: %v", manifestURL, err) // Correct here
				break
			}
			if len(rawObj) == 0 {
				continue
			}

			obj := &unstructured.Unstructured{Object: rawObj}
			mapping, err := mapper.RESTMapping(obj.GroupVersionKind().GroupKind(), obj.GroupVersionKind().Version)
			if err != nil {
				log.Printf("Failed to get REST mapping for object %v: %v", obj.GroupVersionKind(), err)
				continue
			}

			var dri dynamic.ResourceInterface
			if mapping.Scope.Name() == meta.RESTScopeNameNamespace {
				ns := obj.GetNamespace()
				if ns == "" {
					ns = operatorNS
					obj.SetNamespace(ns)
				}
				dri = dynamicClient.Resource(mapping.Resource).Namespace(ns)
			} else {
				dri = dynamicClient.Resource(mapping.Resource)
			}

			err = dri.Delete(ctx, obj.GetName(), metav1.DeleteOptions{})
			if apierrors.IsNotFound(err) {
				log.Printf("Resource %s not found; skipping delete", obj.GetName())
			} else if err != nil {
				log.Printf("Failed to delete resource %s: %v", obj.GetName(), err)
			} else {
				log.Printf("Deleted resource %s", obj.GetName())
			}
		}
	}

	return nil
}
func CleanupAWSCreds(ctx context.Context, kubeClient kubernetes.Interface) error {
	const (
		namespace  = "certman-operator"
		secretName = "aws"
	)

	err := kubeClient.CoreV1().Secrets(namespace).Delete(ctx, secretName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

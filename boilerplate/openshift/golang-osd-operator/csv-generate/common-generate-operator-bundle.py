#!/usr/bin/env python3
#
# Common script to generate OSD operator bundles for publishing to OLM. Copies appropriate files
# into a directory, and composes the ClusterServiceVersion which needs bits and
# pieces of our rbac and deployment files.
#
# Usage ./common-generate-operator-bundle.py \
#   -o OPERATOR_NAME \
#   -d OUTPUT_DIR \
#   -p PREVIOUS_VERSION \
#   -i OPERATOR_IMAGE_URI \
#   -V VERSION_BASE

import datetime
import os
import yaml
import argparse

# The registry is pinned to version 4.7 and only the following resouces are permitted in
# the bundle. The full list can be found at https://github.com/operator-framework/operator-registry/blob/release-4.7/pkg/lib/bundle/supported_resources.go#L4-L19
BUNDLE_PERMITTED_RESOURCES = (
    "ClusterServiceVersion",
    "CustomResourceDefinition",
    "Deployment", # this resource is injected into the CSV directly but not supported by the registry
    "Secret",
    "ClusterRole",
    "ClusterRoleBinding",
    "ConfigMap",
    "ServiceAccount",
    "Service",
    "Role",
    "RoleBinding",
    "PrometheusRule",
    "ServiceMonitor",
    "PodDisruptionBudget",
    "PriorityClass",
    "VerticalPodAutoscaler",
    "ConsoleYamlSample",
)

parser = argparse.ArgumentParser()
parser.add_argument("-o", "--operator-name", type=str, help="Name of the operator", required=True)
parser.add_argument("-d", "--output-dir", type=str, help="Directory for the CSV generation", required=True)
parser.add_argument("-p", "--previous-version", type=str, help="Semver of the version being replaced", required=False)
parser.add_argument("-i", "--operator-image", type=str, help="Base index image to be used", required=True)
parser.add_argument("-V", "--operator-version", type=str, help="The full version of the operator (without the leading `v`): {major}.{minor}.{commit-number}-{hash}", required=True)
args = parser.parse_args()

OPERATOR_NAME   = args.operator_name
outdir          = args.output_dir
prev_version    = args.previous_version
operator_image  = args.operator_image
full_version    = args.operator_version

class UnsupportedRegistryResourceKind(Exception):
    def __init__(self, kind, path):
        super().__init__(
            f"The resource at {path} of kind {kind} is not supported"
        )

class NoServiceAccountSubjectInBinding(Exception):
    def __init__(self, binding):
        super().__init__(
            f"No ServiceAccount Subject in the following Binding:\n{binding}")

class NoDeploymentFound(Exception):
    def __init__(self):
        super().__init__("At least one Deployment is required!")

class MultipleDeploymentsNotSupported(Exception):
    def __init__(self, deployments):
        super().__init__(
            f"Multiple Deployments not supported! Found {len(deployments)}.")

class BindingsNotSupported(Exception):
    def __init__(self, bindings):
        super().__init__(
            "[Cluster]RoleBindings are only supported when they correspond " +
            "to provided [Cluster]Roles. Found the following orphans:\n" +
            f"{bindings}")

class UndefinedCSVNamespace(Exception):
    def __init__(self, operator_name):
        super().__init__(
            f"Namespace not defined for operator {operator_name} in CSV template"
        )

class NoAssociatedRoleBinding(Exception):
    def __init__(name, namespace):
        super.__init__(
            f"The Role {name}/{namespace} does not have an associated RoleBinding"
        )

print("Generating CSV for version: %s" % full_version)

if not os.path.exists(outdir):
    os.mkdir(outdir)

VERSION_DIR = os.path.join(outdir, full_version)
if not os.path.exists(VERSION_DIR):
    os.mkdir(VERSION_DIR)

with open('config/templates/csv-template.yaml', 'r') as stream:
    csv = yaml.safe_load(stream)

# by_kind is a map, keyed by Kind, of all the yaml documents we find
# under the deploy/ directory. We need to load them all before we start
# processing because some (e.g. a ClusterRole and the ServiceAccount in
# its corresponding ClusterRoleBinding) are interdependent.
by_kind = {}
# crb_by_cr is a map, keyed by ClusterRole name, of ClusterRoleBinding
# documents.
crb_by_cr = {}
# rb_by_role is a map, keyed by Role name, of RoleBinding documents
rb_by_role = {}
for d, _, files in os.walk('deploy'):
    for fname in files:
        if not fname.endswith('.yaml'):
            continue
        path = os.path.join(d, fname)
        with open(path, 'r') as stream:
            for doc in yaml.safe_load_all(stream):
                print(
                    f"Loading {doc['kind']} {doc['metadata']['name']} " +
                    f"from {path}")
                if doc['kind'] not in BUNDLE_PERMITTED_RESOURCES:
                    raise UnsupportedRegistryResourceKind(doc['kind'], path)
                if doc['kind'] not in by_kind:
                    by_kind[doc['kind']] = []
                by_kind[doc['kind']].append(doc)
                if doc['kind'] == 'ClusterRoleBinding':
                    crb_by_cr[doc['roleRef']['name']] = doc
                if doc['kind'] == 'RoleBinding':
                    rb_by_role[doc['roleRef']['name']] = doc

def log_resource(resource):
    """Log a message that we're processing the given resource.

    :param resource: A k8s resource definition dict, expected to have at least
        a kind and metadata.name.
    """
    print(f"Processing {resource['kind']} {resource['metadata']['name']}")

def filename_for(resource):
    """Generate a unique base file name for the given resource.

    :param resource: A k8s resource definition dict, expected to have at least
        a kind and metadata.name.
    :return: A base file name (no directory) that should be unique for the
        given resource (assuming the resource is itself unique).
    """
    chunks = [resource['kind']]
    ns = resource['metadata'].get('namespace')
    if ns:
        chunks.append(ns)
    chunks.append(resource['metadata']['name'])
    return '_'.join(chunks) + '.yaml'

def bundle(resource):
    """Create a yaml file for the given resource in the bundle.

    Uses the VERSION_DIR global.

    :param resource: A k8s resource definition dict, expected to have at least
        a kind and metadata.name.
    """
    path = os.path.join(VERSION_DIR, filename_for(resource))
    print(f"Bundling {path}")
    with open(path, 'w') as outfile:
        yaml.dump(resource, outfile, default_flow_style=False)

def discover_service_account(binding):
    """Discover or default the service account in a [Cluster]RoleBinding.

    If binding is None, default to the operator name.
    Uses the OPERATOR_NAME global.

    :param binding: A [Cluster]RoleBinding resource dict.
    :return: A string ServiceAccount name.
    :raise NoServiceAccountSubjectInBinding: if binding does not contain a
        ServiceAccount Subject.
    """
    if not binding:
        print(
            f"  Using default ServiceAccount ({OPERATOR_NAME}).")
        return OPERATOR_NAME
    for subject in binding.get('subjects', []):
        if subject['kind'] == 'ServiceAccount':
            sa = subject['name']
            print(f"  Discovered ServiceAccount {sa}.")
            return sa
    raise NoServiceAccountSubjectInBinding(binding)

def trim_index(index, kind, item):
    """Removes one or all items of a given kind from the index.

    :param index: Dict, keyed by kind, of k8s resources.
    :param kind: String kind name to trim.
    :param item: The item to trim. May be:
        - A dict k8s resource. That specific resource is removed. If not found,
          this function is a no-op.
        - None. This function is a no-op.
        - The string 'ALL'. The entire list for the given kind is removed.
    """
    if kind not in index:
        return
    if not item:
        return
    if item == 'ALL':
        index.pop(kind, None)
        return
    name = item['metadata']['name']
    ns = item['metadata'].get('namespace')
    # Find the specific resource to remove
    for i in range(len(index[kind])):
        meta = index[kind][i]['metadata']
        if meta['name'] == name and meta.get('namespace') == ns:
            found = i
            break
    else:
        # not found
        return
    # Reconstruct the list without the requested item
    index[kind] = index[kind][:found] + index[kind][found+1:]

## Up front sanity checks
if 'Deployment' not in by_kind:
    raise NoDeploymentFound()

# TODO: Should we support additional Deployments that aren't the operator
# Deployment?
if len(by_kind['Deployment']) > 1:
    raise MultipleDeploymentsNotSupported(by_kind['Deployment'])

## Process CRDs
if 'CustomResourceDefinition' in by_kind:
    csv['spec']['customresourcedefinitions'] = {'owned': []}
for crd in by_kind.get('CustomResourceDefinition', []):
    log_resource(crd)

    # And register the CRD as "owned" in the CSV
    for version in crd["spec"]["versions"]:
        csv['spec']['customresourcedefinitions']['owned'].append(
            {
                "name": crd["metadata"]["name"],
                "description": crd["spec"]["names"]["kind"],
                "displayName": crd["spec"]["names"]["kind"],
                "kind": crd["spec"]["names"]["kind"],
                "version": version["name"]
            }
        )
# These will be written to the bundle at the end along with generic resources

## Process [Cluster]Role[Binding]s (TODO: Match up ServiceAccounts)
# Role and ClusterRole are processed similarly
rolekind_csvkey_bindingmap = (
    ('ClusterRole', 'clusterPermissions', crb_by_cr),
    # NOTE: The schema supports `permissions` for Roles, but OLM may not. For
    # backward compatibility, treat Roles and RoleBindings as generic bundle
    # resources.
    # ('Role', 'permissions', rb_by_role),
)
for kind, csvkey, binding_map in rolekind_csvkey_bindingmap:
    if kind in by_kind:
        csv['spec']['install']['spec'][csvkey] = []
    for role in by_kind.get(kind, []):
        log_resource(role)
        # Figure out the ServiceAccount.
        binding = binding_map.get(role['metadata']['name'], None)
        sa = discover_service_account(binding)
        # Get rid of the binding, if found, so it doesn't end up in generic
        # bundle resources.
        trim_index(by_kind, f"{kind}Binding", binding)
        # Inject the rules and ServiceAccount
        csv['spec']['install']['spec'][csvkey].append(
            {
                'rules': role['rules'],
                'serviceAccountName': sa,
            })
    # Get rid of these so we can iterate over what's left at the end
    trim_index(by_kind, kind, 'ALL')


csv['spec']['install']['spec']['permissions'] = []

if 'namespace' not in csv['metadata']:
    raise UndefinedCSVNamespace(OPERATOR_NAME)

# Find namespace of operator from CSV template
namespace = csv['metadata']['namespace']

if 'Role' in by_kind:
    for role in by_kind['Role']:
        # We assume there is always a rolebinding of defined role
        if role['metadata']['name'] not in rb_by_role:
            raise NoAssociatedRoleBinding(role['metadata']['name'], role['metadata']['namespace'])
        role_binding = rb_by_role[role['metadata']['name']]

        # If the RoleBinding subject is a ServiceAccount in the same namespace as the operator
        # we add it to CSV and remove it from the by_kind dict
        if len(role_binding['subjects']) == 1 and \
            role_binding['subjects'][0]['kind'] == 'ServiceAccount' and \
            role_binding['subjects'][0].get('namespace', namespace) == namespace:
                csv['spec']['install']['spec']['permissions'].append(
                    {
                        'rules': role['rules'],
                        'serviceAccountName': role_binding['subjects'][0]['name']
                    }
                )
                trim_index(by_kind, 'Role', role)
                trim_index(by_kind, 'RoleBinding', role_binding)

## Add the Deployment
# We already made sure there's exactly one Deployment
deploy = by_kind['Deployment'][0]
# Use the operator image pull spec we were passed
deploy['spec']['template']['spec']['containers'][0]['image'] = operator_image
# Add or replace OPERATOR_IMAGE env var
env = deploy['spec']['template']['spec']['containers'][0].get('env')
if env:
    # Does OPERATOR_IMAGE key already exist in spec? If so, update value
    for entry in env:
        if entry['name'] == 'OPERATOR_IMAGE':
            entry['value'] = operator_image
            break
    # If not, add it
    else:
        env.append(dict(name='OPERATOR_IMAGE', value=operator_image))
else:
    # The container has no environment variables, so just set this one
    env = dict(name='OPERATOR_IMAGE', value=operator_image)

csv['spec']['install']['spec']['deployments'] = [
    {
        'name': deploy['metadata']['name'],
        'spec': deploy['spec'],
    }
]
# Get rid of these so we can iterate over what's left at the end
trim_index(by_kind, 'Deployment', 'ALL')

# Get rid of ServiceAccounts
# TODO: Sanity check these against "generated" ServiceAccounts
print("Ignoring all explicitly defined ServiceAccounts")
trim_index(by_kind, 'ServiceAccount', 'ALL')

# If there are any bindings left, it's an error
leftover_bindings = []
for kind, _, _ in rolekind_csvkey_bindingmap:
    leftover_bindings.extend(by_kind.get(kind, []))
if leftover_bindings:
    raise BindingsNotSupported(leftover_bindings)

## Process any additional resources
# These don't go in the CSV; they just get copied into the bundle as is.
for kind, docs in by_kind.items():
    for doc in docs:
        bundle(doc)

# Update the versions to include git hash:
csv['metadata']['name'] = f"{OPERATOR_NAME}.v{full_version}"
csv['spec']['version'] = full_version
if prev_version:
    csv['spec']['replaces'] = f"{OPERATOR_NAME}.v{prev_version}"

# Set the CSV createdAt annotation:
now = datetime.datetime.now()
csv['metadata']['annotations']['createdAt'] = now.strftime("%Y-%m-%dT%H:%M:%SZ")

# Write the CSV to disk:
csv_filename = f"{OPERATOR_NAME}.v{full_version}.clusterserviceversion.yaml"
csv_file = os.path.join(VERSION_DIR, csv_filename)
with open(csv_file, 'w') as outfile:
    yaml.dump(csv, outfile, default_flow_style=False)
print("Wrote ClusterServiceVersion: %s" % csv_file)

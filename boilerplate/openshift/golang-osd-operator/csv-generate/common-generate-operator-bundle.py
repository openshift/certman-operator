#!/usr/bin/env python3
#
# Common script to generate OSD operator bundles for publishing to OLM. Copies appropriate files
# into a directory, and composes the ClusterServiceVersion which needs bits and
# pieces of our rbac and deployment files.
#
# Usage ./common-generate-operator-bundle.py -o OPERATOR_NAME -d OUTPUT_DIR -p PREVIOUS_VERSION -n GIT_NUM_COMMITS -c COMMIT_HASH -i HIVE_IMAGE

import datetime
import os
import sys
import yaml
import shutil
import argparse
import string

# This script will append the current number of commits given as an arg
# (presumably since some past base tag), and the git hash arg for a final
# version like: 0.1.189-3f73a592
VERSION_BASE = "0.1"

parser = argparse.ArgumentParser()
parser.add_argument("-o", "--operator-name", type=str, help="Name of the operator", required=True)
parser.add_argument("-d", "--output-dir", type=str, help="Directory for the CSV generation", required=True)
parser.add_argument("-p", "--previous-version", type=str, help="Directory for the CSV generation", required=True)
parser.add_argument("-n", "--commit-number", type=str, help="Number of commits in the project (used for version generation)", required=True)
parser.add_argument("-c", "--commit-hash", type=str, help="Current commit hashDirectory for the CSV generation (used for version generation)", required=True)
parser.add_argument("-i", "--operator-image", type=str, help="Base index image to be used", required=True)
args = parser.parse_args()

operator_name   = args.operator_name
outdir          = args.output_dir
prev_version    = args.previous_version
git_num_commits = args.commit_number
git_hash        = args.commit_hash
operator_image  = args.operator_image

full_version = "%s.%s-%s" % (VERSION_BASE, git_num_commits, git_hash)
print("Generating CSV for version: %s" % full_version)

if not os.path.exists(outdir):
    os.mkdir(outdir)

version_dir = os.path.join(outdir, full_version)
if not os.path.exists(version_dir):
    os.mkdir(version_dir)

with open('config/templates/csv-template.yaml'.format(operator_name), 'r') as stream:
    csv = yaml.load(stream)

csv['spec']['customresourcedefinitions']['owned'] = []

# Copy all CRD files over to the bundle output dir:
crd_files = [ f for f in os.listdir('deploy/crds') if f.endswith('_crd.yaml') ]
for file_name in crd_files:
    full_path = os.path.join('deploy/crds', file_name)
    if (os.path.isfile(os.path.join('deploy/crds', file_name))):
        shutil.copy(full_path, os.path.join(version_dir, file_name))
    # Load CRD so we can use attributes from it
    with open("deploy/crds/{}".format(file_name), "r") as stream:
        crd = yaml.load(stream)
    # Update CSV template customresourcedefinitions key
    csv['spec']['customresourcedefinitions']['owned'].append(
        {
            "name": crd["metadata"]["name"],
            "description": crd["spec"]["names"]["kind"],
            "displayName": crd["spec"]["names"]["kind"],
            "kind": crd["spec"]["names"]["kind"],
            "version": crd["spec"]["version"]
        }
    )

csv['spec']['install']['spec']['clusterPermissions'] = []

# Add operator role to the CSV:
with open('deploy/role.yaml', 'r') as stream:
    operator_role = yaml.load(stream)
    csv['spec']['install']['spec']['clusterPermissions'].append(
        {
            'rules': operator_role['rules'],
            'serviceAccountName': operator_name,
        })

# Add our deployment spec for the operator:
with open('deploy/operator.yaml', 'r') as stream:
    operator_components = []
    operator = yaml.load_all(stream)
    for doc in operator:
        operator_components.append(doc)
    # There is only one yaml document in the operator deployment
    operator_deployment = operator_components[0]
    csv['spec']['install']['spec']['deployments'][0]['spec'] = operator_deployment['spec']

# Update the deployment to use the defined image:
csv['spec']['install']['spec']['deployments'][0]['spec']['template']['spec']['containers'][0]['image'] = operator_image

# Update the versions to include git hash:
csv['metadata']['name'] = "{}.v{}".format(operator_name, full_version)
csv['spec']['version'] = full_version
csv['spec']['replaces'] = "{}.v{}".format(operator_name, prev_version)

# Set the CSV createdAt annotation:
now = datetime.datetime.now()
csv['metadata']['annotations']['createdAt'] = now.strftime("%Y-%m-%dT%H:%M:%SZ")

# Write the CSV to disk:
csv_filename = "{}.v{}.clusterserviceversion.yaml".format(operator_name, full_version)
csv_file = os.path.join(version_dir, csv_filename)
with open(csv_file, 'w') as outfile:
    yaml.dump(csv, outfile, default_flow_style=False)
print("Wrote ClusterServiceVersion: %s" % csv_file)

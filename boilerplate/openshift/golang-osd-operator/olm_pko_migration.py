#!/usr/bin/env python3
"""
Convert OLM manifests to PKO (Package Operator) format.

This script reads Kubernetes manifests from a folder and converts them
to Package Operator format with appropriate phase annotations.
"""

import argparse
import os
import subprocess
import sys
from pathlib import Path
from typing import Any

import yaml


# PKO (Package Operator) Constants
PKO_PHASE_ANNOTATION = "package-operator.run/phase"
PKO_COLLISION_PROTECTION_ANNOTATION = "package-operator.run/collision-protection"
PKO_COLLISION_PROTECTION_VALUE = "IfNoController"

# Phase names
PHASE_CRDS = "crds"
PHASE_NAMESPACE = "namespace"
PHASE_RBAC = "rbac"
PHASE_DEPLOY = "deploy"
PHASE_CLEANUP_RBAC = "cleanup-rbac"
PHASE_CLEANUP_DEPLOY = "cleanup-deploy"


TEKTON_PIPELINE_TEMPLATE = """apiVersion: tekton.dev/v1
kind: PipelineRun
metadata:
  annotations:
    build.appstudio.openshift.io/repo: {github_url}?rev={{{{revision}}}}
    build.appstudio.redhat.com/commit_sha: '{{{{revision}}}}'
    build.appstudio.redhat.com/target_branch: '{{{{target_branch}}}}'
    pipelinesascode.tekton.dev/cancel-in-progress: '{cancel_in_progress}'
    pipelinesascode.tekton.dev/max-keep-runs: '3'
    pipelinesascode.tekton.dev/on-cel-expression: event == "{event}" && target_branch
      == "{default_branch}"
  labels:
    appstudio.openshift.io/application: {operator_name}
    appstudio.openshift.io/component: {operator_name}-pko
    pipelines.appstudio.openshift.io/type: build
  name: {operator_name}-pko-on-{event}
  namespace: {operator_name}-tenant
spec:
  params:
  - name: git-url
    value: '{{{{source_url}}}}'
  - name: revision
    value: '{{{{revision}}}}'
  - name: output-image
    value: quay.io/redhat-user-workloads/{operator_name}-tenant/openshift/{operator_name}-pko:{image_tag}
  - name: dockerfile
    value: build/Dockerfile.pko
  - name: path-context
    value: deploy_pko
  - name: skip-preflight-cert-check
    value: true{additional_params}
  taskRunTemplate:
    serviceAccountName: build-pipeline-{operator_name}-pko
  workspaces:
  - name: git-auth
    secret:
      secretName: '{{{{ git_auth_secret }}}}'
  pipelineRef:
    resolver: git
    params:
    - name: url
      value: https://github.com/openshift/boilerplate
    - name: revision
      value: {boilerplate_branch}
    - name: pathInRepo
      value: pipelines/docker-build-oci-ta/pipeline.yaml
status: {{}}
"""


DOCKERFILE_TEMPLATE = """FROM scratch

LABEL com.redhat.component="openshift-{operator_name}" \\
      io.k8s.description="{operator_name} Operator for OpenShift Dedicated" \\
      description="{operator_name} Operator for OpenShift Dedicated" \\
      distribution-scope="public" \\
      name="{github_subpath}" \\
      url="{github_url}" \\
      vendor="Red Hat, Inc." \\
      release="v0.0.0" \\
      version="v0.0.0"

COPY * /package/
"""


CLEANUP_JOB_TEMPLATE = """---
# This Job cleans up old OLM resources after migrating to PKO
# IMPORTANT: Review and customize this template before deploying!
# 
# Things to customize:
# 1. Adjust the namespace if needed
# 2. Modify resource filters (CSV names, labels, etc.)
# 3. Review RBAC permissions
# 4. Update the cleanup logic for your specific operator
#
apiVersion: v1
kind: ServiceAccount
metadata:
  name: olm-cleanup
  namespace: openshift-{operator_name}
  annotations:
    package-operator.run/phase: cleanup-rbac
    package-operator.run/collision-protection: IfNoController
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: olm-cleanup
  namespace: openshift-{operator_name}
  annotations:
    package-operator.run/phase: cleanup-rbac
    package-operator.run/collision-protection: IfNoController
rules:
  # CUSTOMIZE: Adjust permissions as needed for your cleanup tasks
  - apiGroups:
      - operators.coreos.com
    resources:
      - clusterserviceversions
      - subscriptions
    verbs:
      - list
      - get
      - watch
      - delete
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: olm-cleanup
  namespace: openshift-{operator_name}
  annotations:
    package-operator.run/phase: cleanup-rbac
    package-operator.run/collision-protection: IfNoController
roleRef:
  kind: Role
  name: olm-cleanup
  apiGroup: rbac.authorization.k8s.io
subjects:
  - kind: ServiceAccount
    name: olm-cleanup
    namespace: openshift-{operator_name}
---
apiVersion: batch/v1
kind: Job
metadata:
  name: olm-cleanup
  namespace: openshift-{operator_name}
  annotations:
    package-operator.run/phase: cleanup-deploy
    package-operator.run/collision-protection: IfNoController
spec:
  ttlSecondsAfterFinished: 100
  template:
    metadata:
      annotations:
        openshift.io/required-scc: restricted-v2
    spec:
      serviceAccountName: olm-cleanup
      priorityClassName: openshift-user-critical
      restartPolicy: Never
      containers:
        - name: delete-csv
          image: image-registry.openshift-image-registry.svc:5000/openshift/cli:latest
          imagePullPolicy: Always
          command:
            - sh
            - -c
            - |
              #!/bin/sh
              set -euxo pipefail
              # CUSTOMIZE: Update the label selector for your operator
              # Example pattern: operators.coreos.com/OPERATOR_NAME.NAMESPACE
              oc -n openshift-{operator_name} delete csv -l "operators.coreos.com/{operator_name}.openshift-{operator_name}" || true
              
              # CUSTOMIZE: Add any additional cleanup logic here
              # Examples:
              # - Delete subscriptions
              # - Delete operator groups
              # - Clean up custom resources
          resources:
            requests:
              cpu: 100m
              memory: 100Mi
"""


CLUSTERPACKAGE_TEMPLATE = """apiVersion: v1
kind: Template
parameters:
  - name: CHANNEL
    required: false
  - name: PKO_IMAGE
    required: true
  - name: IMAGE_TAG
    required: true
  - name: IMAGE_DIGEST
    required: true
  - name: NAMESPACE
    value: openshift-{operator_name}
  - name: REPO_NAME
    value: {operator_name}
    required: true
metadata:
  name: selectorsyncset-template
objects:
  - apiVersion: hive.openshift.io/v1
    kind: SelectorSyncSet
    metadata:
      labels:
        managed.openshift.io/gitHash: ${{IMAGE_TAG}}
        managed.openshift.io/gitRepoName: ${{REPO_NAME}}
        managed.openshift.io/osd: "true"
      name: {operator_name}-stage
    spec:
      clusterDeploymentSelector:
        matchLabels:
          api.openshift.com/managed: "true"
      resourceApplyMode: Sync
      resources:
        - apiVersion: package-operator.run/v1alpha1
          kind: ClusterPackage
          metadata:
            name: "${{REPO_NAME}}"
            annotations:
              package-operator.run/collision-protection: IfNoController
          spec:
            image: ${{PKO_IMAGE}}:${{IMAGE_TAG}}
"""

def get_remotes() -> list[str]:
    """Get list of git remote URLs."""
    try:
        # First check if we're in a git repository
        result = subprocess.run(
            ["git", "rev-parse", "--git-dir"],
            capture_output=True,
            text=True,
            check=True
        )
    except subprocess.CalledProcessError:
        raise RuntimeError(
            "Not in a git repository. This script must be run from within a git repository."
        )
    
    try:
        result = subprocess.run(
            ["git", "remote", "-v"],
            capture_output=True,
            text=True,
            check=True
        )
        # Parse output: "origin\turl (fetch)" -> extract just the URLs
        remotes = []
        for line in result.stdout.strip().splitlines():
            parts = line.split()
            if len(parts) >= 2:
                remotes.append(parts[1])
        # Remove duplicates (fetch and push are listed separately)
        return list(dict.fromkeys(remotes))
    except subprocess.CalledProcessError as e:
        raise RuntimeError(
            f"Could not get git remotes: {e.stderr if e.stderr else str(e)}"
        )


def get_github_url() -> str:
    """Get GitHub URL from git remotes, preferring openshift remotes."""
    remotes = get_remotes()
    for remote in remotes:
        if 'openshift' not in remote:
            continue
        
        if remote.startswith('http'):
            return remote.removesuffix(".git")
        elif ":" in remote:
            # Handle git@github.com:org/repo format
            subpath = remote.split(":")[1]
            subpath = subpath.removesuffix(".git")
            return f"https://github.com/{subpath}"
        else:
            raise RuntimeError(
                f"Cannot parse git remote URL format: {remote}. Expected 'https://...' or 'git@github.com:...'"
            )
    
    raise RuntimeError(
        "Could not find an 'openshift' git remote. Available remotes: " + 
        (", ".join(remotes) if remotes else "(none)")
    )
            

def get_operator_name() -> str:
    """Extract operator name from git remote URL."""
    try:
        remotes = get_remotes()
        if not remotes:
            return "unknown-operator"
        
        # Use the first remote URL
        url = remotes[0]
        
        # Remove .git suffix if present
        if url.endswith(".git"):
            url = url[:-4]
        
        # Extract the last part of the path
        # Works for both https://github.com/org/repo and git@github.com:org/repo
        if "/" in url:
            return url.split("/")[-1]
        elif ":" in url:
            return url.split(":")[-1].split("/")[-1]
        
        return "unknown-operator"
    except Exception as e:
        print(f"Warning: Could not extract operator name: {e}", file=sys.stderr)
        return "unknown-operator"


def get_default_branch() -> str:
    """
    Detect the default branch name for the current repository.
    
    Returns:
        str: The default branch name ('main' or 'master')
    
    Raises:
        RuntimeError: If not in a git repository or cannot determine default branch
    """
    try:
        # First check if we're in a git repository
        result = subprocess.run(
            ["git", "rev-parse", "--git-dir"],
            capture_output=True,
            text=True,
            check=True
        )
    except subprocess.CalledProcessError:
        raise RuntimeError(
            "Not in a git repository. This script must be run from within a git repository."
        )
    
    try:
        # Try to get the default branch from the remote
        result = subprocess.run(
            ["git", "symbolic-ref", "refs/remotes/origin/HEAD"],
            capture_output=True,
            text=True,
            check=False
        )
        
        if result.returncode == 0:
            # Output format: "refs/remotes/origin/main" or "refs/remotes/origin/master"
            branch = result.stdout.strip().split("/")[-1]
            if branch in ("main", "master"):
                return branch
        
        # Fallback: check current branch
        result = subprocess.run(
            ["git", "branch", "--show-current"],
            capture_output=True,
            text=True,
            check=True
        )
        
        current_branch = result.stdout.strip()
        if current_branch in ("main", "master"):
            return current_branch
        
        # Last resort: check if main or master branches exist locally
        result = subprocess.run(
            ["git", "branch", "--list"],
            capture_output=True,
            text=True,
            check=True
        )
        
        branches = [line.strip().lstrip("* ") for line in result.stdout.splitlines()]
        if "main" in branches:
            return "main"
        if "master" in branches:
            return "master"
        
        # Default to 'main' if we can't determine
        print("Warning: Could not determine default branch, defaulting to 'main'", file=sys.stderr)
        return "main"
        
    except subprocess.CalledProcessError as e:
        print(f"Warning: Error detecting default branch: {e.stderr if e.stderr else str(e)}", file=sys.stderr)
        print("Defaulting to 'main'", file=sys.stderr)
        return "main"


def get_pko_manifest(operator_name: str) -> dict[str, Any]:
    """Generate the PKO PackageManifest structure."""
    return {
        "apiVersion": "manifests.package-operator.run/v1alpha1",
        "kind": "PackageManifest",
        "metadata": {
            "name": operator_name,
            "annotations": {"name": "annotation"},
        },
        "spec": {
            "scopes": ["Cluster"],
            "phases": [
                {"name": PHASE_CRDS},
                {"name": PHASE_NAMESPACE},
                {"name": PHASE_RBAC},
                {"name": PHASE_DEPLOY},
                {"name": PHASE_CLEANUP_RBAC},
                {"name": PHASE_CLEANUP_DEPLOY},
            ],
            "availabilityProbes": [
                {
                    "probes": [
                        {"condition": {"type": "Available", "status": "True"}},
                        {
                            "fieldsEqual": {
                                "fieldA": ".status.updatedReplicas",
                                "fieldB": ".status.replicas",
                            }
                        },
                    ],
                    "selector": {"kind": {"group": "apps", "kind": "Deployment"}}
                },
            ],
            "config": {
                "openAPIV3Schema": {
                    "properties": {
                        "image": {
                            "description": "Operator image to deploy",
                            "type": "string",
                            "default": "None",
                        },
                    "type": "object",
                    }
                }
            },
        },
    }


def get_manifest_files(path: str, recursive: bool = True) -> list[Path]:
    """
    Get all YAML/YML files from the given path.
    
    Args:
        path: Directory path to search
        recursive: If True, search subdirectories recursively
    
    Returns:
        list of Path objects for YAML files
    """
    path_obj = Path(path)
    if not path_obj.exists():
        return []
    
    yaml_extensions = {'.yaml', '.yml'}
    
    if recursive:
        # Recursively find all YAML files
        yaml_files = []
        for ext in yaml_extensions:
            yaml_files.extend(path_obj.rglob(f'*{ext}'))
        return [f for f in yaml_files if f.is_file()]
    else:
        # Only get files in the immediate directory
        return [
            f for f in path_obj.iterdir()
            if f.is_file() and f.suffix in yaml_extensions
        ]


def load_manifests(path: str, recursive: bool = True) -> list[str]:
    """
    Load all manifest files as strings.
    
    Args:
        path: Directory path containing manifests
        recursive: If True, search subdirectories recursively
    
    Returns:
        list of manifest file contents as strings
    """
    files = get_manifest_files(path, recursive=recursive)
    manifests = []
    for file in sorted(files):  # Sort for consistent ordering
        try:
            print(f"Reading: {file}")
            with open(file, "r") as f:
                manifests.append(f.read())
        except (IOError, OSError) as e:
            print(f"Warning: Could not read {file}: {e}", file=sys.stderr)
    return manifests


def annotate(manifest: dict[str, Any], annotation: str) -> dict[str, Any]:
    """Add PKO phase annotations to a manifest."""
    if "metadata" not in manifest:
        manifest["metadata"] = {}
    if "annotations" not in manifest["metadata"]:
        manifest["metadata"]["annotations"] = {}

    manifest["metadata"]["annotations"][PKO_PHASE_ANNOTATION] = annotation
    manifest["metadata"]["annotations"][PKO_COLLISION_PROTECTION_ANNOTATION] = PKO_COLLISION_PROTECTION_VALUE

    return manifest


def set_image_template(manifest: dict[str, Any]) -> dict[str, Any]:
    """Replace container images with template variable."""
    try:
        containers = manifest["spec"]["template"]["spec"]["containers"]
        for container in containers:
            container["image"] = "{{ .config.image }}"
    except (KeyError, TypeError):
        pass
    return manifest


def annotate_manifests(manifests: list[str]) -> list[dict[str, Any]]:
    """Parse and annotate all manifests based on their kind."""
    annotated = []

    for manifest_str in manifests:
        try:
            manifest = yaml.safe_load(manifest_str)
            if not manifest or not isinstance(manifest, dict):
                continue

            kind = manifest.get("kind")

            if kind == "CustomResourceDefinition":
                annotated.append(annotate(manifest, PHASE_CRDS))
            elif kind in ["ClusterRole", "ClusterRoleBinding"]:
                annotated.append(annotate(manifest, PHASE_RBAC))
            elif kind in ["Role", "RoleBinding"]:
                annotated.append(annotate(manifest, PHASE_RBAC))
            elif kind == "ServiceAccount":
                annotated.append(annotate(manifest, PHASE_RBAC))
            elif kind == "Service":
                annotated.append(annotate(manifest, PHASE_DEPLOY))
            elif kind == "Deployment":
                manifest = annotate(manifest, PHASE_DEPLOY)
                manifest = set_image_template(manifest)
                annotated.append(manifest)
            elif kind == "ServiceMonitor":
                annotated.append(annotate(manifest, PHASE_DEPLOY))
            else:
                print(f"Unhandled type: {kind}")
                annotated.append(manifest)

        except yaml.YAMLError as e:
            print(f"Error parsing manifest: {e}", file=sys.stderr)
            continue

    return annotated


def write_manifest(
    manifest: dict[str, Any], directory: str, filename: str = None, force: bool = False
) -> None:
    """
    Write a manifest to a YAML file.
    
    Args:
        manifest: The manifest dictionary to write
        directory: Target directory
        filename: Optional filename (auto-generated if not provided)
        force: If True, skip the kind filter check
    """
    kind = manifest.get("kind")
    name = manifest.get("metadata", {}).get("name", "unknown")

    # Skip certain kinds (unless forced)
    if not force and kind in [None, "ClusterPackage", "Package", "PackageManifest"]:
        return

    # Create directory if it doesn't exist
    dir_path = Path(directory)
    dir_path.mkdir(parents=True, exist_ok=True)

    # Generate filename
    if filename is None:
        ext = ".yaml.gotmpl" if kind == "Deployment" else ".yaml"
        filename = f"{kind}-{name}{ext}"

    filepath = dir_path / filename
    print(f"Writing {kind} to {filepath}")

    with open(filepath, "w") as f:
        yaml.dump(manifest, f, default_flow_style=False, sort_keys=False)


def write_pko_dockerfile():
    operator_name = get_operator_name()
    operator_upstream = get_github_url()
    build_folder = Path("./build")
    if not build_folder.exists():
        raise RuntimeError(
            f"Operator does not contain a ./build folder. Expected path: {build_folder.absolute()}"
        )
    pko_manifest = build_folder / "Dockerfile.pko"
    with open(pko_manifest, mode="w") as manifest:
        manifest.write(
            DOCKERFILE_TEMPLATE.format(
                operator_name = operator_name,
                github_url = operator_upstream,
                github_subpath = operator_upstream.removeprefix("https://github.com/")
            )
        )

def write_clusterpackage_template():
    """Write the ClusterPackage template to hack/pko/clusterpackage.yaml."""
    operator_name = get_operator_name()
    pko_hack_folder = Path("./hack/pko")
    pko_hack_folder.mkdir(parents=True, exist_ok=True)
    
    clusterpackage_file = pko_hack_folder / "clusterpackage.yaml"
    print(f"Writing ClusterPackage template to {clusterpackage_file}")
    with open(clusterpackage_file, "w") as f:
        f.write(CLUSTERPACKAGE_TEMPLATE.format(operator_name=operator_name))


def write_tekton_pipelines():
    operator_name = get_operator_name()
    operator_upstream = get_github_url()
    default_branch = get_default_branch()
    
    tekton_folder = Path("./.tekton")
    if not tekton_folder.exists():
        raise RuntimeError(
            f"Operator does not contain a .tekton folder. Expected path: {tekton_folder.absolute()}"
        )
    push_manifest = tekton_folder / (operator_name + "-pko-push.yaml")
    pr_manifest = tekton_folder / (operator_name + "-pko-pull-request.yaml")
    
    # Detect boilerplate branch - try to use the same as the current repo, fallback to master
    # since the boilerplate repo still uses master as default
    boilerplate_branch = "master"  # boilerplate repo uses master
    
    # Push pipeline - no additional params, standard revision tag
    with open(push_manifest, mode="w") as manifest:
        manifest.write(
            TEKTON_PIPELINE_TEMPLATE.format(
                operator_name = operator_name,
                github_url = operator_upstream,
                cancel_in_progress = "false",
                event = "push",
                image_tag = "{{revision}}",
                additional_params = "",
                default_branch = default_branch,
                boilerplate_branch = boilerplate_branch
            )
        )
    
    # Pull request pipeline - add image-expires-after param and prefix revision with 'on-pr-'
    pr_additional_params = """
  - name: image-expires-after
    value: 3d"""
    
    with open(pr_manifest, mode="w") as manifest:
        manifest.write(
            TEKTON_PIPELINE_TEMPLATE.format(
                operator_name = operator_name,
                github_url = operator_upstream,
                cancel_in_progress = "true",
                event = "pull_request",
                image_tag = "on-pr-{{revision}}",
                additional_params = pr_additional_params,
                default_branch = default_branch,
                boilerplate_branch = boilerplate_branch
            )
        )


def modify_manifests(path: str, output_dir: str = "deploy_pko", recursive: bool = True) -> None:
    """
    Main function to convert manifests from OLM to PKO format.
    
    Args:
        path: Source directory containing manifests
        output_dir: Output directory for PKO manifests
        recursive: If True, search subdirectories recursively
    """
    operator_name = get_operator_name()
    pko_dir = Path(output_dir)

    print(f"Scanning {'recursively' if recursive else 'non-recursively'} in: {path}")
    print("-" * 60)

    # Load and process manifests
    manifests = load_manifests(path, recursive=recursive)
    
    if not manifests:
        print(f"Warning: No YAML manifests found in {path}")
        return
    
    print(f"\nProcessing {len(manifests)} manifest(s)...")
    print("-" * 60)
    
    annotated = annotate_manifests(manifests)

    # Write processed manifests
    for manifest in annotated:
        write_manifest(manifest, str(pko_dir))

    # Write PKO manifest
    print("-" * 60)
    pko_manifest = get_pko_manifest(operator_name)
    write_manifest(pko_manifest, str(pko_dir), "manifest.yaml", force=True)
    
    # Write cleanup Job template
    cleanup_file = pko_dir / "Cleanup-OLM-Job.yaml"
    print(f"Writing cleanup Job template to {cleanup_file}")
    with open(cleanup_file, "w") as f:
        f.write(CLEANUP_JOB_TEMPLATE.format(operator_name=operator_name))

    print("-" * 60)
    print(f"\nConversion complete! PKO manifests written to {pko_dir}")
    print(f"Total manifests processed: {len(annotated)}")
    print()
    print("IMPORTANT: A cleanup Job template has been generated:")
    print(f"  {cleanup_file}")
    print()
    print("Please review and customize this file before deploying it to your cluster.")
    print("This Job will clean up old OLM resources (CSVs, Subscriptions, etc.)")
    print("The cleanup resources use phases 'cleanup-rbac' and 'cleanup-deploy'.")
    print()
    print("Next steps:")
    print("  1. Ensure a Konflux tenant matching the Tekton pipelines exists")
    print("  2. Update the SAAS files that might deploy your operator")


def print_migration_info():
    """Print information about what this migration script will do."""
    print()
    print("=" * 80)
    print("OLM to PKO (Package Operator) Migration Script")
    print("=" * 80)
    print()
    print("This script will help you migrate your operator from OLM to PKO format.")
    print()
    print("The following will be generated:")
    print("  • PKO manifests with proper phase annotations (crds, rbac, deploy)")
    print("  • PackageManifest with phases including cleanup-rbac and cleanup-deploy")
    print("  • Dockerfile.pko for building the PKO package (in build/ folder)")
    print("  • Tekton pipeline manifests for CI/CD (in .tekton/ folder)")
    print("  • Cleanup-OLM-Job.yaml template for removing old OLM resources")
    print("  • ClusterPackage template for deployment (in hack/pko/ folder)")
    print()
    print("IMPORTANT - Post-Migration Steps:")
    print("  After migration, you MUST review and deploy the generated")
    print("  Cleanup-OLM-Job.yaml file to clean up old OLM resources")
    print("  (CSVs, Subscriptions, etc.) from your clusters.")
    print()
    print("=" * 80)
    print()


def main():
    """CLI entry point."""
    parser = argparse.ArgumentParser(
        description="Convert OLM manifests to PKO (Package Operator) format"
    )
    parser.add_argument(
        "-f",
        "--folder",
        default="deploy",
        help="Folder that contains the source manifests (default: deploy)",
    )
    parser.add_argument(
        "-o",
        "--output",
        default="deploy_pko",
        help="Output folder for PKO manifests (default: deploy_pko)",
    )
    parser.add_argument(
        "--no-recursive",
        action="store_true",
        help="Only process files in the top-level folder, not subdirectories",
    )
    parser.add_argument(
        "--no-dockerfile",
        action="store_true",
        help="Skip generating Dockerfile.pko in build/ folder",
    )
    parser.add_argument(
        "--no-tekton",
        action="store_true",
        help="Skip generating Tekton pipeline manifests in .tekton/ folder",
    )
    parser.add_argument(
        "-y",
        "--yes",
        action="store_true",
        help="Skip confirmation prompt and proceed automatically",
    )

    args = parser.parse_args()

    # Print migration information and get confirmation
    if not args.yes:
        print_migration_info()
        try:
            response = input("Do you want to proceed with the migration? (y/N): ").strip().lower()
            if response not in ['y', 'yes']:
                print("\nMigration cancelled.")
                sys.exit(0)
        except (KeyboardInterrupt, EOFError):
            print("\n\nMigration cancelled.")
            sys.exit(0)
        print()

    if not Path(args.folder).exists():
        print(f"Error: Folder '{args.folder}' does not exist!", file=sys.stderr)
        sys.exit(1)

    modify_manifests(args.folder, output_dir=args.output, recursive=not args.no_recursive)

    if not args.no_dockerfile:
        print("\nGenerating Dockerfile.pko...")
        try:
            write_pko_dockerfile()
        except Exception as e:
            print(f"Warning: Could not generate Dockerfile: {e}", file=sys.stderr)
    else:
        print("\nSkipping Dockerfile generation (--no-dockerfile)")

    if not args.no_tekton:
        print("\nGenerating Tekton pipeline manifests...")
        try:
            write_tekton_pipelines()
        except Exception as e:
            print(f"Warning: Could not generate Tekton pipelines: {e}", file=sys.stderr)
    else:
        print("\nSkipping Tekton pipeline generation (--no-tekton)")

    print("\nGenerating ClusterPackage template...")
    try:
        write_clusterpackage_template()
    except Exception as e:
        print(f"Warning: Could not generate ClusterPackage template: {e}", file=sys.stderr)


if __name__ == "__main__":
    main()

#!/usr/bin/env python3
"""
Unit tests for olm_pko_migration.py

Run with: python3 -m pytest test_olm_pko_migration.py -v
"""

import os
import shutil
import subprocess
import tempfile
import unittest
from pathlib import Path
from unittest.mock import MagicMock, Mock, patch, mock_open

import yaml

# Import the module under test
import olm_pko_migration as migration


class TestGitOperations(unittest.TestCase):
    """Test git-related helper functions."""

    @patch('subprocess.run')
    def test_get_remotes_success(self, mock_run):
        """Test successful git remote parsing."""
        mock_run.side_effect = [
            # First call: git rev-parse --git-dir
            Mock(returncode=0, stdout='.git\n', stderr=''),
            # Second call: git remote -v
            Mock(
                returncode=0,
                stdout='origin\tgit@github.com:openshift/my-operator.git (fetch)\n'
                       'origin\tgit@github.com:openshift/my-operator.git (push)\n',
                stderr=''
            )
        ]
        
        remotes = migration.get_remotes()
        self.assertEqual(remotes, ['git@github.com:openshift/my-operator.git'])

    @patch('subprocess.run')
    def test_get_remotes_not_git_repo(self, mock_run):
        """Test get_remotes fails when not in a git repository."""
        mock_run.side_effect = subprocess.CalledProcessError(
            returncode=128,
            cmd=['git', 'rev-parse', '--git-dir'],
            stderr='fatal: not a git repository'
        )
        
        with self.assertRaises(RuntimeError) as ctx:
            migration.get_remotes()
        self.assertIn('Not in a git repository', str(ctx.exception))

    @patch('olm_pko_migration.get_remotes')
    def test_get_github_url_ssh_format(self, mock_get_remotes):
        """Test GitHub URL extraction from SSH format."""
        mock_get_remotes.return_value = ['git@github.com:openshift/my-operator.git']
        
        url = migration.get_github_url()
        self.assertEqual(url, 'https://github.com/openshift/my-operator')

    @patch('olm_pko_migration.get_remotes')
    def test_get_github_url_https_format(self, mock_get_remotes):
        """Test GitHub URL extraction from HTTPS format."""
        mock_get_remotes.return_value = ['https://github.com/openshift/my-operator.git']
        
        url = migration.get_github_url()
        self.assertEqual(url, 'https://github.com/openshift/my-operator')

    @patch('olm_pko_migration.get_remotes')
    def test_get_github_url_no_openshift_remote(self, mock_get_remotes):
        """Test error when no openshift remote is found."""
        mock_get_remotes.return_value = ['https://github.com/other-org/repo.git']
        
        with self.assertRaises(RuntimeError) as ctx:
            migration.get_github_url()
        self.assertIn('Could not find an', str(ctx.exception))

    @patch('olm_pko_migration.get_remotes')
    def test_get_operator_name_from_url(self, mock_get_remotes):
        """Test operator name extraction from git URL."""
        mock_get_remotes.return_value = ['https://github.com/openshift/my-operator.git']
        
        name = migration.get_operator_name()
        self.assertEqual(name, 'my-operator')

    @patch('olm_pko_migration.get_remotes')
    def test_get_operator_name_ssh_format(self, mock_get_remotes):
        """Test operator name extraction from SSH format."""
        mock_get_remotes.return_value = ['git@github.com:openshift/test-operator.git']
        
        name = migration.get_operator_name()
        self.assertEqual(name, 'test-operator')

    @patch('subprocess.run')
    def test_get_default_branch_from_remote_head(self, mock_run):
        """Test detecting default branch from remote HEAD."""
        mock_run.side_effect = [
            # First call: git rev-parse --git-dir
            Mock(returncode=0, stdout='.git\n', stderr=''),
            # Second call: git symbolic-ref refs/remotes/origin/HEAD
            Mock(returncode=0, stdout='refs/remotes/origin/main\n', stderr='')
        ]
        
        branch = migration.get_default_branch()
        self.assertEqual(branch, 'main')

    @patch('subprocess.run')
    def test_get_default_branch_master_from_remote_head(self, mock_run):
        """Test detecting master branch from remote HEAD."""
        mock_run.side_effect = [
            # First call: git rev-parse --git-dir
            Mock(returncode=0, stdout='.git\n', stderr=''),
            # Second call: git symbolic-ref refs/remotes/origin/HEAD
            Mock(returncode=0, stdout='refs/remotes/origin/master\n', stderr='')
        ]
        
        branch = migration.get_default_branch()
        self.assertEqual(branch, 'master')

    @patch('subprocess.run')
    def test_get_default_branch_from_current_branch(self, mock_run):
        """Test detecting default branch from current branch when remote HEAD fails."""
        mock_run.side_effect = [
            # First call: git rev-parse --git-dir
            Mock(returncode=0, stdout='.git\n', stderr=''),
            # Second call: git symbolic-ref refs/remotes/origin/HEAD (fails)
            Mock(returncode=128, stdout='', stderr='fatal: ref refs/remotes/origin/HEAD is not a symbolic ref'),
            # Third call: git branch --show-current
            Mock(returncode=0, stdout='main\n', stderr='')
        ]
        
        branch = migration.get_default_branch()
        self.assertEqual(branch, 'main')

    @patch('subprocess.run')
    def test_get_default_branch_from_branch_list(self, mock_run):
        """Test detecting default branch from branch list when other methods fail."""
        mock_run.side_effect = [
            # First call: git rev-parse --git-dir
            Mock(returncode=0, stdout='.git\n', stderr=''),
            # Second call: git symbolic-ref refs/remotes/origin/HEAD (fails)
            Mock(returncode=128, stdout='', stderr='fatal: ref refs/remotes/origin/HEAD is not a symbolic ref'),
            # Third call: git branch --show-current (returns feature branch)
            Mock(returncode=0, stdout='feature-branch\n', stderr=''),
            # Fourth call: git branch --list
            Mock(returncode=0, stdout='  feature-branch\n* main\n  develop\n', stderr='')
        ]
        
        branch = migration.get_default_branch()
        self.assertEqual(branch, 'main')

    @patch('subprocess.run')
    def test_get_default_branch_defaults_to_main(self, mock_run):
        """Test that get_default_branch defaults to 'main' when detection fails."""
        mock_run.side_effect = [
            # First call: git rev-parse --git-dir
            Mock(returncode=0, stdout='.git\n', stderr=''),
            # Second call: git symbolic-ref refs/remotes/origin/HEAD (fails)
            Mock(returncode=128, stdout='', stderr='fatal: ref refs/remotes/origin/HEAD is not a symbolic ref'),
            # Third call: git branch --show-current
            Mock(returncode=0, stdout='feature-branch\n', stderr=''),
            # Fourth call: git branch --list (no main or master)
            Mock(returncode=0, stdout='  feature-branch\n  develop\n', stderr='')
        ]
        
        branch = migration.get_default_branch()
        self.assertEqual(branch, 'main')

    @patch('subprocess.run')
    def test_get_default_branch_not_git_repo(self, mock_run):
        """Test get_default_branch fails when not in a git repository."""
        mock_run.side_effect = subprocess.CalledProcessError(
            returncode=128,
            cmd=['git', 'rev-parse', '--git-dir'],
            stderr='fatal: not a git repository'
        )
        
        with self.assertRaises(RuntimeError) as ctx:
            migration.get_default_branch()
        self.assertIn('Not in a git repository', str(ctx.exception))


class TestManifestAnnotation(unittest.TestCase):
    """Test manifest annotation functions."""

    def test_annotate_adds_phase_annotation(self):
        """Test that annotate adds the correct phase annotation."""
        manifest = {
            'apiVersion': 'v1',
            'kind': 'ServiceAccount',
            'metadata': {'name': 'test-sa'}
        }
        
        result = migration.annotate(manifest, migration.PHASE_RBAC)
        
        self.assertIn('annotations', result['metadata'])
        self.assertEqual(
            result['metadata']['annotations'][migration.PKO_PHASE_ANNOTATION],
            migration.PHASE_RBAC
        )
        self.assertEqual(
            result['metadata']['annotations'][migration.PKO_COLLISION_PROTECTION_ANNOTATION],
            migration.PKO_COLLISION_PROTECTION_VALUE
        )

    def test_annotate_preserves_existing_annotations(self):
        """Test that annotate preserves existing annotations."""
        manifest = {
            'metadata': {
                'name': 'test',
                'annotations': {'existing': 'value'}
            }
        }
        
        result = migration.annotate(manifest, migration.PHASE_DEPLOY)
        
        self.assertEqual(result['metadata']['annotations']['existing'], 'value')
        self.assertIn(migration.PKO_PHASE_ANNOTATION, result['metadata']['annotations'])

    def test_set_image_template_replaces_image(self):
        """Test that set_image_template replaces container images."""
        manifest = {
            'spec': {
                'template': {
                    'spec': {
                        'containers': [
                            {'name': 'operator', 'image': 'quay.io/openshift/operator:v1.0'},
                            {'name': 'sidecar', 'image': 'quay.io/openshift/sidecar:latest'}
                        ]
                    }
                }
            }
        }
        
        result = migration.set_image_template(manifest)
        
        for container in result['spec']['template']['spec']['containers']:
            self.assertEqual(container['image'], '{{ .config.image }}')

    def test_set_image_template_handles_missing_containers(self):
        """Test that set_image_template handles manifests without containers."""
        manifest = {'spec': {}}
        
        # Should not raise an exception
        result = migration.set_image_template(manifest)
        self.assertEqual(result, manifest)


class TestManifestProcessing(unittest.TestCase):
    """Test manifest file processing functions."""

    def test_annotate_manifests_crds(self):
        """Test that CRDs are annotated with crds phase."""
        manifest_str = """
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: testkinds.mygroup.com
spec:
  group: mygroup.com
"""
        
        result = migration.annotate_manifests([manifest_str])
        
        self.assertEqual(len(result), 1)
        self.assertEqual(
            result[0]['metadata']['annotations'][migration.PKO_PHASE_ANNOTATION],
            migration.PHASE_CRDS
        )

    def test_annotate_manifests_rbac_resources(self):
        """Test that RBAC resources are annotated with rbac phase."""
        rbac_manifests = [
            """
apiVersion: v1
kind: ServiceAccount
metadata:
  name: test-sa
""",
            """
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: test-role
""",
            """
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: test-role
""",
        ]
        
        results = migration.annotate_manifests(rbac_manifests)
        
        self.assertEqual(len(results), 3)
        for result in results:
            self.assertEqual(
                result['metadata']['annotations'][migration.PKO_PHASE_ANNOTATION],
                migration.PHASE_RBAC
            )

    def test_annotate_manifests_deployment(self):
        """Test that Deployments are annotated with deploy phase and image templated."""
        manifest_str = """
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-deployment
spec:
  template:
    spec:
      containers:
      - name: operator
        image: quay.io/openshift/test:v1.0
"""
        
        result = migration.annotate_manifests([manifest_str])
        
        self.assertEqual(len(result), 1)
        self.assertEqual(
            result[0]['metadata']['annotations'][migration.PKO_PHASE_ANNOTATION],
            migration.PHASE_DEPLOY
        )
        self.assertEqual(
            result[0]['spec']['template']['spec']['containers'][0]['image'],
            '{{ .config.image }}'
        )

    def test_annotate_manifests_skips_invalid_yaml(self):
        """Test that invalid YAML is skipped gracefully."""
        manifests = [
            "valid:\n  yaml: true",
            "invalid: yaml: broken:",
            "another:\n  valid: manifest"
        ]
        
        # Should not raise an exception
        result = migration.annotate_manifests(manifests)
        # Should process the valid ones
        self.assertGreaterEqual(len(result), 2)


class TestFileDiscovery(unittest.TestCase):
    """Test file discovery functions."""

    def setUp(self):
        """Create a temporary directory structure for testing."""
        self.temp_dir: str = tempfile.mkdtemp()
        self.addCleanup(lambda: shutil.rmtree(self.temp_dir))
        
        # Create test directory structure
        # temp_dir/
        #   ├── deploy/
        #   │   ├── deployment.yaml
        #   │   ├── service.yaml
        #   │   └── crds/
        #   │       └── crd.yaml
        #   └── other.txt
        
        deploy_dir = Path(self.temp_dir) / 'deploy'
        deploy_dir.mkdir()
        
        (deploy_dir / 'deployment.yaml').write_text('apiVersion: apps/v1\nkind: Deployment')
        (deploy_dir / 'service.yml').write_text('apiVersion: v1\nkind: Service')
        
        crds_dir = deploy_dir / 'crds'
        crds_dir.mkdir()
        (crds_dir / 'crd.yaml').write_text('apiVersion: apiextensions.k8s.io/v1')
        
        (Path(self.temp_dir) / 'other.txt').write_text('not yaml')

    def test_get_manifest_files_recursive(self):
        """Test recursive manifest file discovery."""
        files = migration.get_manifest_files(
            str(Path(self.temp_dir) / 'deploy'),
            recursive=True
        )
        
        self.assertEqual(len(files), 3)
        filenames = {f.name for f in files}
        self.assertEqual(filenames, {'deployment.yaml', 'service.yml', 'crd.yaml'})

    def test_get_manifest_files_non_recursive(self):
        """Test non-recursive manifest file discovery."""
        files = migration.get_manifest_files(
            str(Path(self.temp_dir) / 'deploy'),
            recursive=False
        )
        
        self.assertEqual(len(files), 2)
        filenames = {f.name for f in files}
        self.assertEqual(filenames, {'deployment.yaml', 'service.yml'})

    def test_get_manifest_files_nonexistent_path(self):
        """Test that nonexistent paths return empty list."""
        files = migration.get_manifest_files('/nonexistent/path')
        self.assertEqual(files, [])

    def test_load_manifests(self):
        """Test loading manifest contents."""
        manifests = migration.load_manifests(
            str(Path(self.temp_dir) / 'deploy'),
            recursive=False
        )
        
        self.assertEqual(len(manifests), 2)
        # Check that content was actually loaded
        for manifest in manifests:
            self.assertIn('apiVersion', manifest)


class TestPKOManifestGeneration(unittest.TestCase):
    """Test PKO-specific manifest generation."""

    @patch('olm_pko_migration.get_operator_name')
    def test_get_pko_manifest_structure(self, mock_get_name):
        """Test that PKO PackageManifest has correct structure."""
        mock_get_name.return_value = 'test-operator'
        
        manifest = migration.get_pko_manifest('test-operator')
        
        self.assertEqual(manifest['apiVersion'], 'manifests.package-operator.run/v1alpha1')
        self.assertEqual(manifest['kind'], 'PackageManifest')
        self.assertEqual(manifest['metadata']['name'], 'test-operator')
        
        # Check phases
        phase_names = [p['name'] for p in manifest['spec']['phases']]
        expected_phases = [
            migration.PHASE_CRDS,
            migration.PHASE_NAMESPACE,
            migration.PHASE_RBAC,
            migration.PHASE_DEPLOY,
            migration.PHASE_CLEANUP_RBAC,
            migration.PHASE_CLEANUP_DEPLOY,
        ]
        self.assertEqual(phase_names, expected_phases)
        
        # Check availability probes exist
        self.assertIn('availabilityProbes', manifest['spec'])
        self.assertGreater(len(manifest['spec']['availabilityProbes']), 0)
        
        # Check config schema
        self.assertIn('config', manifest['spec'])
        self.assertIn('openAPIV3Schema', manifest['spec']['config'])


class TestFileWriting(unittest.TestCase):
    """Test file writing functions."""

    def setUp(self):
        """Create a temporary directory for output."""
        self.temp_dir = tempfile.mkdtemp()
        self.addCleanup(lambda: shutil.rmtree(self.temp_dir))

    def test_write_manifest_creates_file(self):
        """Test that write_manifest creates the expected file."""
        manifest = {
            'apiVersion': 'v1',
            'kind': 'ServiceAccount',
            'metadata': {'name': 'test-sa'}
        }
        
        migration.write_manifest(manifest, self.temp_dir)
        
        expected_file = Path(self.temp_dir) / 'ServiceAccount-test-sa.yaml'
        self.assertTrue(expected_file.exists())
        
        # Verify content is valid YAML
        with open(expected_file) as f:
            loaded = yaml.safe_load(f)
        self.assertEqual(loaded['kind'], 'ServiceAccount')

    def test_write_manifest_deployment_uses_gotmpl(self):
        """Test that Deployment manifests use .gotmpl extension."""
        manifest = {
            'apiVersion': 'apps/v1',
            'kind': 'Deployment',
            'metadata': {'name': 'test-deploy'}
        }
        
        migration.write_manifest(manifest, self.temp_dir)
        
        expected_file = Path(self.temp_dir) / 'Deployment-test-deploy.yaml.gotmpl'
        self.assertTrue(expected_file.exists())

    def test_write_manifest_custom_filename(self):
        """Test writing manifest with custom filename."""
        manifest = {
            'apiVersion': 'v1',
            'kind': 'Service',
            'metadata': {'name': 'test'}
        }
        
        migration.write_manifest(manifest, self.temp_dir, filename='custom.yaml')
        
        expected_file = Path(self.temp_dir) / 'custom.yaml'
        self.assertTrue(expected_file.exists())

    def test_write_manifest_skips_package_kinds(self):
        """Test that certain kinds are skipped unless forced."""
        manifest = {
            'apiVersion': 'v1',
            'kind': 'ClusterPackage',
            'metadata': {'name': 'test'}
        }
        
        migration.write_manifest(manifest, self.temp_dir)
        
        # Should not create any files
        files = list(Path(self.temp_dir).iterdir())
        self.assertEqual(len(files), 0)

    def test_write_manifest_force_writes_package_kinds(self):
        """Test that force=True allows writing package kinds."""
        manifest = {
            'apiVersion': 'manifests.package-operator.run/v1alpha1',
            'kind': 'PackageManifest',
            'metadata': {'name': 'test'}
        }
        
        migration.write_manifest(manifest, self.temp_dir, filename='test.yaml', force=True)
        
        expected_file = Path(self.temp_dir) / 'test.yaml'
        self.assertTrue(expected_file.exists())


class TestTemplateGeneration(unittest.TestCase):
    """Test template generation functions."""

    def setUp(self):
        """Create a temporary directory structure."""
        self.temp_dir = tempfile.mkdtemp()
        self.addCleanup(lambda: shutil.rmtree(self.temp_dir))
        
        # Change to temp dir for git operations
        self.original_dir = os.getcwd()
        os.chdir(self.temp_dir)
        
        # Initialize git repo
        subprocess.run(['git', 'init', '-b', 'main'], check=True, capture_output=True)
        subprocess.run(['git', 'config', 'user.name', 'Test'], check=True, capture_output=True)
        subprocess.run(['git', 'config', 'user.email', 'test@example.com'], check=True, capture_output=True)
        subprocess.run(
            ['git', 'remote', 'add', 'origin', 'https://github.com/openshift/test-operator.git'],
            check=True,
            capture_output=True
        )

    def tearDown(self):
        """Return to original directory."""
        os.chdir(self.original_dir)

    def test_write_pko_dockerfile(self):
        """Test PKO Dockerfile generation."""
        # Create build directory
        build_dir = Path(self.temp_dir) / 'build'
        build_dir.mkdir()
        
        migration.write_pko_dockerfile()
        
        dockerfile = build_dir / 'Dockerfile.pko'
        self.assertTrue(dockerfile.exists())
        
        content = dockerfile.read_text()
        self.assertIn('FROM scratch', content)
        self.assertIn('openshift-test-operator', content)
        self.assertIn('COPY * /package/', content)

    def test_write_pko_dockerfile_no_build_folder(self):
        """Test error when build folder doesn't exist."""
        with self.assertRaises(RuntimeError) as ctx:
            migration.write_pko_dockerfile()
        self.assertIn('does not contain a ./build folder', str(ctx.exception))

    def test_write_tekton_pipelines(self):
        """Test Tekton pipeline generation."""
        # Create .tekton directory
        tekton_dir = Path(self.temp_dir) / '.tekton'
        tekton_dir.mkdir()
        
        migration.write_tekton_pipelines()
        
        push_pipeline = tekton_dir / 'test-operator-pko-push.yaml'
        pr_pipeline = tekton_dir / 'test-operator-pko-pull-request.yaml'
        
        self.assertTrue(push_pipeline.exists())
        self.assertTrue(pr_pipeline.exists())
        
        # Check push pipeline content
        push_content = push_pipeline.read_text()
        self.assertIn('apiVersion: tekton.dev/v1', push_content)
        self.assertIn('event == "push"', push_content)
        self.assertNotIn('image-expires-after', push_content)
        # Verify it uses the detected default branch (main in this test)
        self.assertIn('target_branch\n      == "main"', push_content)
        # Verify it uses master for boilerplate
        self.assertIn('value: master', push_content)
        
        # Check PR pipeline content
        pr_content = pr_pipeline.read_text()
        self.assertIn('event == "pull_request"', pr_content)
        self.assertIn('image-expires-after', pr_content)
        self.assertIn('on-pr-', pr_content)
        # Verify it uses the detected default branch (main in this test)
        self.assertIn('target_branch\n      == "main"', pr_content)
        # Verify it uses master for boilerplate
        self.assertIn('value: master', pr_content)

    def test_write_clusterpackage_template(self):
        """Test ClusterPackage template generation."""
        migration.write_clusterpackage_template()
        
        clusterpackage_file = Path(self.temp_dir) / 'hack' / 'pko' / 'clusterpackage.yaml'
        self.assertTrue(clusterpackage_file.exists())
        
        content = clusterpackage_file.read_text()
        self.assertIn('kind: Template', content)
        self.assertIn('kind: SelectorSyncSet', content)
        self.assertIn('kind: ClusterPackage', content)
        self.assertIn('test-operator', content)


class TestIntegration(unittest.TestCase):
    """Integration tests for the full conversion process."""

    def setUp(self):
        """Create a temporary directory with sample manifests."""
        self.temp_dir = tempfile.mkdtemp()
        self.addCleanup(lambda: shutil.rmtree(self.temp_dir))
        
        # Change to temp dir
        self.original_dir = os.getcwd()
        os.chdir(self.temp_dir)
        
        # Initialize git repo
        subprocess.run(['git', 'init', '-b', 'main'], check=True, capture_output=True)
        subprocess.run(['git', 'config', 'user.name', 'Test'], check=True, capture_output=True)
        subprocess.run(['git', 'config', 'user.email', 'test@example.com'], check=True, capture_output=True)
        subprocess.run(
            ['git', 'remote', 'add', 'origin', 'https://github.com/openshift/test-operator.git'],
            check=True,
            capture_output=True
        )
        
        # Create deploy directory with sample manifests
        deploy_dir = Path(self.temp_dir) / 'deploy'
        deploy_dir.mkdir()
        
        # CRD
        (deploy_dir / 'crd.yaml').write_text("""
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: tests.example.com
spec:
  group: example.com
""")
        
        # ServiceAccount
        (deploy_dir / 'serviceaccount.yaml').write_text("""
apiVersion: v1
kind: ServiceAccount
metadata:
  name: test-operator
""")
        
        # Deployment
        (deploy_dir / 'deployment.yaml').write_text("""
apiVersion: apps/v1
kind: Deployment
metadata:
  name: test-operator
spec:
  template:
    spec:
      containers:
      - name: operator
        image: quay.io/openshift/test-operator:latest
""")

    def tearDown(self):
        """Return to original directory."""
        os.chdir(self.original_dir)

    def test_modify_manifests_end_to_end(self):
        """Test complete manifest conversion process."""
        output_dir = 'deploy_pko'
        
        migration.modify_manifests('deploy', output_dir=output_dir, recursive=True)
        
        output_path = Path(self.temp_dir) / output_dir
        
        # Check that output directory was created
        self.assertTrue(output_path.exists())
        
        # Check for PackageManifest
        manifest_file = output_path / 'manifest.yaml'
        self.assertTrue(manifest_file.exists())
        
        with open(manifest_file) as f:
            package_manifest = yaml.safe_load(f)
        self.assertEqual(package_manifest['kind'], 'PackageManifest')
        
        # Check for converted manifests
        crd_file = output_path / 'CustomResourceDefinition-tests.example.com.yaml'
        self.assertTrue(crd_file.exists())
        
        # Verify CRD has correct phase annotation
        with open(crd_file) as f:
            crd = yaml.safe_load(f)
        self.assertEqual(
            crd['metadata']['annotations'][migration.PKO_PHASE_ANNOTATION],
            migration.PHASE_CRDS
        )
        
        # Check deployment has .gotmpl extension and templated image
        deployment_files = list(output_path.glob('Deployment-*.yaml.gotmpl'))
        self.assertEqual(len(deployment_files), 1)
        
        with open(deployment_files[0]) as f:
            deployment = yaml.safe_load(f)
        self.assertEqual(
            deployment['spec']['template']['spec']['containers'][0]['image'],
            '{{ .config.image }}'
        )
        
        # Check cleanup job was created
        cleanup_file = output_path / 'Cleanup-OLM-Job.yaml'
        self.assertTrue(cleanup_file.exists())


if __name__ == '__main__':
    unittest.main()

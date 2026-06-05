#!/usr/bin/env python3
"""
Fetch Prow CI job artifacts from Google Cloud Storage.
"""

import argparse
import json
import os
import re
import subprocess
import sys
from pathlib import Path
from urllib.parse import urlparse


def parse_prow_url(url):
    """
    Parse Prow job URL to extract GCS path components.

    Returns dict with:
        - gcs_base_path: Full GCS path (gs://...)
        - bucket_path: Path within bucket
        - build_id: Numeric build ID
        - job_name: Name of the Prow job
    """
    # Handle both gcsweb URLs and direct GCS URLs
    if 'test-platform-results' not in url:
        raise ValueError("URL must contain 'test-platform-results'")

    # Extract path after test-platform-results
    match = re.search(r'test-platform-results/(.+?)(?:\?|$)', url)
    if not match:
        raise ValueError("Could not parse test-platform-results path")

    bucket_path = match.group(1).rstrip('/')

    # Extract build ID (10+ digits)
    build_match = re.search(r'/(\d{10,})/?', bucket_path)
    if not build_match:
        raise ValueError("Could not find build ID (10+ digits) in URL")

    build_id = build_match.group(1)

    # Extract job name (segment before build_id)
    job_match = re.search(r'/([^/]+)/\d{10,}/?', bucket_path)
    if not job_match:
        raise ValueError("Could not extract job name from URL")

    job_name = job_match.group(1)

    gcs_base_path = f"gs://test-platform-results/{bucket_path}"

    return {
        'gcs_base_path': gcs_base_path,
        'bucket_path': bucket_path,
        'build_id': build_id,
        'job_name': job_name
    }


def download_from_gcs(gcs_path, local_path):
    """Download a file from GCS using gcloud storage cp."""
    try:
        os.makedirs(os.path.dirname(local_path), exist_ok=True)
        cmd = [
            'gcloud', 'storage', 'cp',
            gcs_path,
            local_path,
            '--no-user-output-enabled'
        ]
        subprocess.run(cmd, check=True, capture_output=True)
        return True
    except subprocess.CalledProcessError as e:
        print(f"Warning: Could not download {gcs_path}: {e.stderr.decode()}", file=sys.stderr)
        return False


def fetch_prowjob_json(gcs_base_path, output_dir):
    """Fetch prowjob.json and return parsed JSON."""
    gcs_path = f"{gcs_base_path}/prowjob.json"
    local_path = os.path.join(output_dir, 'prowjob.json')

    if download_from_gcs(gcs_path, local_path):
        try:
            with open(local_path, 'r') as f:
                return json.load(f)
        except json.JSONDecodeError as e:
            print(f"Error: Could not parse JSON from {local_path}: {e}", file=sys.stderr)
            return None
    return None


def fetch_build_log(gcs_base_path, output_dir):
    """Fetch build-log.txt."""
    gcs_path = f"{gcs_base_path}/build-log.txt"
    local_path = os.path.join(output_dir, 'build-log.txt')
    return download_from_gcs(gcs_path, local_path)




def main():
    parser = argparse.ArgumentParser(description='Fetch Prow CI job artifacts from GCS')
    parser.add_argument('url', help='Prow job URL (gcsweb or direct GCS)')
    parser.add_argument('-o', '--output', default='.work/prow-artifacts',
                        help='Output directory (default: .work/prow-artifacts)')

    args = parser.parse_args()

    # Parse URL
    try:
        parsed = parse_prow_url(args.url)
    except ValueError as e:
        print(f"Error: {e}", file=sys.stderr)
        return 1

    print(f"Prow Job: {parsed['job_name']}")
    print(f"Build ID: {parsed['build_id']}")
    print(f"GCS Path: {parsed['gcs_base_path']}")
    print()

    # Create output directory
    output_dir = os.path.join(args.output, parsed['build_id'])
    os.makedirs(output_dir, exist_ok=True)

    # Track failures
    had_errors = False

    # Fetch prowjob.json (optional - don't fail if missing)
    print("Fetching prowjob.json...")
    prowjob = fetch_prowjob_json(parsed['gcs_base_path'], output_dir)
    if prowjob is not None:
        print("✓ prowjob.json downloaded")
    else:
        print("⚠ Could not fetch prowjob.json (optional artifact)")

    # Fetch build-log.txt
    print("Fetching build-log.txt...")
    if fetch_build_log(parsed['gcs_base_path'], output_dir):
        print("✓ build-log.txt downloaded")
    else:
        print("✗ Could not fetch build-log.txt")
        had_errors = True

    print(f"\nArtifacts saved to: {output_dir}")

    return 1 if had_errors else 0


if __name__ == '__main__':
    sys.exit(main())

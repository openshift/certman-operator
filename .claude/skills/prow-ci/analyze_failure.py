#!/usr/bin/env python3
"""
Analyze Prow CI job failures from downloaded artifacts.
"""

import argparse
import json
import os
import re
import sys
from pathlib import Path


def analyze_build_log(log_file):
    """Analyze build-log.txt for common failure patterns."""
    if not os.path.exists(log_file):
        return None

    analysis = {
        'errors': [],
        'failures': [],
        'warnings': [],
        'patterns': {}
    }

    # Common failure patterns (compiled for efficiency)
    patterns = {
        'compilation_error': re.compile(r'(?:compilation failed|build failed|cannot find package)', re.IGNORECASE),
        'test_failure': re.compile(r'(?:FAIL:|Test failed:|tests failed)', re.IGNORECASE),
        'lint_error': re.compile(r'(?:golangci-lint|gofmt|go vet) .* failed', re.IGNORECASE),
        'timeout': re.compile(r'(?:timeout|timed out|deadline exceeded)', re.IGNORECASE),
        'oom': re.compile(r'(?:out of memory|OOMKilled|killed by signal)', re.IGNORECASE),
        'image_pull': re.compile(r'(?:Failed to pull image|ErrImagePull|ImagePullBackOff)', re.IGNORECASE),
        'permission_denied': re.compile(r'(?:permission denied|forbidden|unauthorized)', re.IGNORECASE),
    }

    # Initialize pattern counters
    for pattern_name in patterns:
        analysis['patterns'][pattern_name] = 0

    # Stream and process line-by-line to avoid memory pressure
    with open(log_file, 'r', encoding='utf-8', errors='replace') as f:
        for line in f:
            line_stripped = line.strip()

            # Count pattern matches
            for pattern_name, pattern_regex in patterns.items():
                if pattern_regex.search(line):
                    analysis['patterns'][pattern_name] += 1

            # Extract error lines (limit collection to avoid memory issues)
            # Use independent if statements to allow capturing multiple categories per line
            if len(analysis['errors']) < 10 and re.search(r'\bERROR\b', line, re.IGNORECASE):
                analysis['errors'].append(line_stripped)
            if len(analysis['failures']) < 10 and re.search(r'\bFAIL(ED)?\b', line, re.IGNORECASE):
                analysis['failures'].append(line_stripped)
            if len(analysis['warnings']) < 5 and re.search(r'\bWARNING\b', line, re.IGNORECASE):
                analysis['warnings'].append(line_stripped)

    # Remove patterns with zero occurrences
    analysis['patterns'] = {k: v for k, v in analysis['patterns'].items() if v > 0}

    return analysis


def analyze_prowjob(prowjob_file):
    """Extract key information from prowjob.json."""
    if not os.path.exists(prowjob_file):
        return None

    try:
        with open(prowjob_file, 'r') as f:
            data = json.load(f)
    except (json.JSONDecodeError, OSError) as e:
        print(f"Error: Could not parse prowjob from {prowjob_file}: {e}", file=sys.stderr)
        return None

    status = data.get('status', {})
    spec = data.get('spec', {})

    return {
        'state': status.get('state', 'unknown'),
        'start_time': status.get('startTime'),
        'completion_time': status.get('completionTime'),
        'url': status.get('url', ''),
        'job_name': spec.get('job', 'unknown'),
        'type': spec.get('type', 'unknown'),
        'refs': spec.get('refs', {}),
    }


def generate_analysis_report(artifacts_dir):
    """Generate comprehensive failure analysis report."""
    report = {
        'prowjob': None,
        'build_log': None,
        'summary': ''
    }

    # Analyze prowjob.json
    prowjob_file = os.path.join(artifacts_dir, 'prowjob.json')
    report['prowjob'] = analyze_prowjob(prowjob_file)

    # Analyze build-log.txt
    build_log_file = os.path.join(artifacts_dir, 'build-log.txt')
    report['build_log'] = analyze_build_log(build_log_file)

    # Generate summary
    summary_parts = []

    if report['prowjob']:
        pj = report['prowjob']
        summary_parts.append(f"Job: {pj['job_name']}")
        summary_parts.append(f"State: {pj['state']}")

    if report['build_log'] and report['build_log']['patterns']:
        summary_parts.append("\nDetected Patterns:")
        for pattern, count in report['build_log']['patterns'].items():
            summary_parts.append(f"  - {pattern}: {count} occurrences")

    if report['build_log'] and report['build_log']['errors']:
        summary_parts.append(f"\nTop Errors ({len(report['build_log']['errors'])}):")
        for err in report['build_log']['errors'][:3]:
            summary_parts.append(f"  - {err[:150]}")

    report['summary'] = '\n'.join(summary_parts)

    return report


def format_markdown_report(report):
    """Format analysis as Markdown."""
    lines = ["# Prow CI Failure Analysis\n"]

    if report['prowjob']:
        pj = report['prowjob']
        lines.append("## Job Information")
        lines.append(f"- **Job**: {pj['job_name']}")
        lines.append(f"- **State**: {pj['state']}")
        lines.append(f"- **Type**: {pj['type']}")
        if pj.get('url'):
            lines.append(f"- **URL**: {pj['url']}")
        lines.append("")

    if report['build_log']:
        bl = report['build_log']

        if bl['patterns']:
            lines.append("## Detected Patterns")
            for pattern, count in sorted(bl['patterns'].items(), key=lambda x: x[1], reverse=True):
                lines.append(f"- **{pattern}**: {count} occurrences")
            lines.append("")

        if bl['errors']:
            lines.append("## Errors")
            for err in bl['errors']:
                lines.append(f"- {err}")
            lines.append("")

        if bl['failures']:
            lines.append("## Failures")
            for fail in bl['failures'][:5]:
                lines.append(f"- {fail}")
            lines.append("")

    return '\n'.join(lines)


def main():
    parser = argparse.ArgumentParser(description='Analyze Prow CI job failures')
    parser.add_argument('artifacts_dir', help='Directory containing downloaded artifacts')
    parser.add_argument('-f', '--format', choices=['text', 'json', 'markdown'],
                        default='markdown', help='Output format')
    parser.add_argument('-o', '--output', help='Output file (default: stdout)')

    args = parser.parse_args()

    if not os.path.exists(args.artifacts_dir):
        print(f"Error: Artifacts directory not found: {args.artifacts_dir}", file=sys.stderr)
        return 1

    # Generate analysis
    report = generate_analysis_report(args.artifacts_dir)

    # Fail fast if build log is missing (required artifact)
    if report.get('build_log') is None:
        print(f"Error: Missing required build-log.txt in {args.artifacts_dir}", file=sys.stderr)
        print("The artifacts directory must contain build-log.txt for analysis.", file=sys.stderr)
        return 1

    # Format output
    if args.format == 'json':
        output = json.dumps(report, indent=2)
    elif args.format == 'markdown':
        output = format_markdown_report(report)
    else:  # text
        output = report['summary']

    # Write output
    if args.output:
        with open(args.output, 'w') as f:
            f.write(output)
        print(f"Analysis saved to: {args.output}")
    else:
        print(output)

    return 0


if __name__ == '__main__':
    sys.exit(main())

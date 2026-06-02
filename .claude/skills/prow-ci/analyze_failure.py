#!/usr/bin/env python3
"""
Analyze Prow CI job failures from downloaded artifacts.
"""

import argparse
import json
import os
import re
import sys
import xml.etree.ElementTree as ET
from pathlib import Path


def parse_junit_xml(xml_file):
    """Parse JUnit XML and extract failures."""
    try:
        tree = ET.parse(xml_file)
        root = tree.getroot()

        failures = []
        # Handle root-level testsuite or nested testsuites
        suites = [root] if root.tag == 'testsuite' else []
        suites.extend(root.findall('.//testsuite'))

        for testsuite in suites:
            suite_name = testsuite.get('name', 'unknown')
            for testcase in testsuite.findall('.//testcase'):
                test_name = testcase.get('name', 'unknown')
                classname = testcase.get('classname', '')

                failure = testcase.find('failure')
                error = testcase.find('error')

                if failure is not None:
                    failures.append({
                        'type': 'failure',
                        'suite': suite_name,
                        'test': test_name,
                        'class': classname,
                        'message': failure.get('message', ''),
                        'details': failure.text or ''
                    })
                elif error is not None:
                    failures.append({
                        'type': 'error',
                        'suite': suite_name,
                        'test': test_name,
                        'class': classname,
                        'message': error.get('message', ''),
                        'details': error.text or ''
                    })

        return failures
    except Exception as e:
        print(f"Warning: Could not parse {xml_file}: {e}", file=sys.stderr)
        return []


def analyze_build_log(log_file):
    """Analyze build-log.txt for common failure patterns."""
    if not os.path.exists(log_file):
        return None

    with open(log_file, 'r', encoding='utf-8', errors='replace') as f:
        content = f.read()

    analysis = {
        'errors': [],
        'failures': [],
        'warnings': [],
        'patterns': {}
    }

    # Common failure patterns
    patterns = {
        'compilation_error': r'(?:compilation failed|build failed|cannot find package)',
        'test_failure': r'(?:FAIL:|Test failed:|tests failed)',
        'lint_error': r'(?:golangci-lint|gofmt|go vet) .* failed',
        'timeout': r'(?:timeout|timed out|deadline exceeded)',
        'oom': r'(?:out of memory|OOMKilled|killed by signal)',
        'image_pull': r'(?:Failed to pull image|ErrImagePull|ImagePullBackOff)',
        'permission_denied': r'(?:permission denied|forbidden|unauthorized)',
    }

    for pattern_name, regex in patterns.items():
        matches = re.findall(regex, content, re.IGNORECASE)
        if matches:
            analysis['patterns'][pattern_name] = len(matches)

    # Extract error lines
    for line in content.splitlines():
        if re.search(r'\bERROR\b', line, re.IGNORECASE):
            analysis['errors'].append(line.strip())
        elif re.search(r'\bFAIL(ED)?\b', line):
            analysis['failures'].append(line.strip())
        elif re.search(r'\bWARNING\b', line, re.IGNORECASE):
            analysis['warnings'].append(line.strip())

    # Limit to most relevant
    analysis['errors'] = analysis['errors'][:10]
    analysis['failures'] = analysis['failures'][:10]
    analysis['warnings'] = analysis['warnings'][:5]

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
        'junit_failures': [],
        'summary': ''
    }

    # Analyze prowjob.json
    prowjob_file = os.path.join(artifacts_dir, 'prowjob.json')
    report['prowjob'] = analyze_prowjob(prowjob_file)

    # Analyze build-log.txt
    build_log_file = os.path.join(artifacts_dir, 'build-log.txt')
    report['build_log'] = analyze_build_log(build_log_file)

    # Analyze JUnit XML files
    artifacts_path = os.path.join(artifacts_dir, 'artifacts')
    if os.path.exists(artifacts_path):
        for xml_file in Path(artifacts_path).rglob('junit*.xml'):
            failures = parse_junit_xml(xml_file)
            report['junit_failures'].extend(failures)

    # Generate summary
    summary_parts = []

    if report['prowjob']:
        pj = report['prowjob']
        summary_parts.append(f"Job: {pj['job_name']}")
        summary_parts.append(f"State: {pj['state']}")

    if report['junit_failures']:
        summary_parts.append(f"\nJUnit Failures: {len(report['junit_failures'])}")
        for f in report['junit_failures'][:5]:
            summary_parts.append(f"  - {f['test']}: {f['message'][:100]}")

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

    if report['junit_failures']:
        lines.append("## Test Failures")
        lines.append(f"\nTotal failures: {len(report['junit_failures'])}\n")
        for f in report['junit_failures']:
            lines.append(f"### {f['test']}")
            lines.append(f"**Suite**: {f['suite']}")
            lines.append(f"**Type**: {f['type']}")
            if f['message']:
                lines.append(f"**Message**: {f['message']}")
            if f['details']:
                lines.append("```")
                lines.append(f['details'][:500])
                if len(f['details']) > 500:
                    lines.append("... (truncated)")
                lines.append("```")
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

import yaml
import sys

file_path = sys.argv[1]
with open(file_path, 'r') as f:
    data = yaml.safe_load(f)

spec = data.get('spec', {})

# Remove pipelineSpec and taskRunSpecs
spec.pop('pipelineSpec', None)
spec.pop('taskRunSpecs', None)

# Add pipelineRef
spec['pipelineRef'] = {
    'resolver': 'git',
    'params': [
        {'name': 'url', 'value': 'https://github.com/openshift/boilerplate'},
        {'name': 'revision', 'value': 'master'},
        {'name': 'pathInRepo', 'value': 'pipelines/docker-build-oci-ta/pipeline.yaml'}
    ]
}

# Write back
with open(file_path, 'w') as f:
    yaml.dump(data, f, default_flow_style=False, sort_keys=False)

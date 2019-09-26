# Usage
# python validate_yaml.py path/to/file/or/dir

import sys
import yaml
from os import listdir
from os.path import isdir, isfile, join, splitext

usage = 'usage: python validate_yaml.py path/to/file/or/dir'

if len(sys.argv) != 2:
    print(usage)
    sys.exit(1)

path = sys.argv[1]

if isfile(path):
    files = [path]
elif isdir(path):
    files = [join(path, f)
             for f in listdir(path)
             if isfile(join(path, f))]
else:
    print(usage)
    sys.exit(1)

error = False
for file_path in files:
    _, ext = splitext(file_path)
    if ext not in ['.yml', '.yaml']:
        continue

    print('validating {}'.format(file_path))
    with open(file_path, 'r') as f:
        data = f.read()
    try:
        yaml.safe_load(data)
    except Exception as e:
        print(e)
        error = True

sys.exit(error)

# Usage
# python validate-yaml.py path/to/file/or/dir

import sys
import yaml
from os import listdir
from os.path import isdir, isfile, join, splitext

usage = "Usage: {0:s} path/to/file/or/dir...".format(sys.argv[0])

if len(sys.argv) < 2:
    print(usage)
    sys.exit(0)

input_paths = sys.argv[1:]

error = False

for path in input_paths:
    if isfile(path):
        files = [path]
    elif isdir(path):
        files = [join(path, f) for f in listdir(path) if isfile(join(path, f))]
    else:
        print("Path {0:s} does not exist".format(path))
        error=True
        continue

    for file_path in files:
        _, ext = splitext(file_path)
        if ext not in [".yml", ".yaml"]:
            continue

        print("Validating YAML {}".format(file_path))
        with open(file_path, "r") as f:
            data = f.read()
        try:
            for y in yaml.safe_load_all(data):
                pass
        except Exception as e:
            print(e)
            error = True

sys.exit(error)

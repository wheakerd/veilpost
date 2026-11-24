"""Compatibility entry point for the frontend package's dev:* commands."""
import argparse
import subprocess
import sys
from project import lifecycle

if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['start', 'stop', 'restart', 'status', 'logs'])
    try:
        sys.exit(lifecycle(parser.parse_args().action, selection='frontend'))
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        print(f'ERROR: {error}', file=sys.stderr)
        sys.exit(1)

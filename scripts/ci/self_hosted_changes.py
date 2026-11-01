#!/usr/bin/env python3
"""Select Docker validation from a NUL-delimited git diff of the whole PR."""
import sys


def requires_validation(paths):
    prefixes = (
        b"apps/api/", b"apps/web/", b"deployments/self-hosted/",
        b"scripts/ci/", b".github/workflows/",
    )
    return any(path == b".dockerignore" or path.startswith(prefixes) for path in paths)


if __name__ == "__main__":
    changed = requires_validation(sys.stdin.buffer.read().split(b"\0"))
    print(f"self-hosted={'true' if changed else 'false'}")

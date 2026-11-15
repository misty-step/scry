#!/usr/bin/env python3
"""The pinned retained Go source the revival inspector derives its schema from.

Only the archive and its checksum live here; nothing builds or runs Go. The
archive holds exactly PATHS at REVISION, and its checksum is unchanged from the
retired US-001.1 migration run that first pinned it.
"""
import subprocess

REVISION = "c6ba395a63814c1f973d8cda5057d54fbb587e9b"
ARCHIVE_SHA256 = "5787d0a07c4d28730f259649b50e53a9a16e5d468437fb69203acf97ea91222b"
PATHS = ("go.mod", "go.sum", "internal/store", "internal/learning")


def archive(root):
    return subprocess.check_output(
        ["git", "-C", str(root), "archive", "--format=tar", REVISION, *PATHS]
    )

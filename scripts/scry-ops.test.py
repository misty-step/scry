#!/usr/bin/env python3
"""Private environment loading remains shared by host and Cloudflare recovery."""
import os
from pathlib import Path
import sys
import tempfile

sys.path.insert(0, str(Path(__file__).resolve().parent / "lib"))
from scry_ops import OperationError, private_env, write_env  # noqa: E402



def fail(message):
    raise AssertionError(message)




def test_private_env_does_not_execute():
    with tempfile.TemporaryDirectory() as tmp:
        tmp = Path(tmp)
        env = tmp / "scry.env"
        fd = os.open(env, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        with os.fdopen(fd, "w") as stream:
            stream.write("FOO=$(id)\n")
            stream.write("BAR=plain\n")
        values = private_env(env)
        if values["FOO"] != "$(id)" or values["BAR"] != "plain":
            fail("environment load did not keep literal values")
        os.chmod(env, 0o644)
        try:
            private_env(env)
        except OperationError:
            pass
        else:
            fail("mode 0644 environment file was accepted")




def test_write_env_roundtrip():
    with tempfile.TemporaryDirectory() as tmp:
        path = Path(tmp) / "env"
        write_env(path, {"HOST": "127.0.0.1", "TOKEN": "abc_def"})
        values = private_env(path)
        if values != {"HOST": "127.0.0.1", "TOKEN": "abc_def"}:
            fail(values)


def main():
    test_private_env_does_not_execute()
    test_write_env_roundtrip()
    print("OK scry-ops")


if __name__ == "__main__":
    main()

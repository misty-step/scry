#!/usr/bin/env python3
"""Explicitly destructive drill for a disposable, already running preview."""
import json, os, subprocess, time, urllib.request, urllib.error
url = os.environ["SCRY_DRILL_URL"].rstrip("/")
if ":18080" in url:
    raise SystemExit("refusing protected preview")
state = os.environ["SCRY_WATCH_STATE"]
def incidents():
    with open(state) as f:
        return json.load(f)["Incidents"] or []
def present(value):
    return value and not value.startswith("0001-")
before = incidents()
assert not any(not present(i["Resolved"]) for i in before), "open incident exists"
req = urllib.request.Request(url + "/ops/fault-drill", method="POST",
    headers={"Authorization": "Bearer " + os.environ["SCRY_FAULT_DRILL_TOKEN"]})
try:
    urllib.request.urlopen(req, timeout=15)
    raise AssertionError("drill did not fail")
except urllib.error.HTTPError as e:
    assert e.code == 500
def wait(check):
    deadline = time.monotonic() + 240
    while time.monotonic() < deadline:
        rows = incidents()
        assert len(rows) <= len(before) + 1, "duplicate incident"
        if len(rows) == len(before) + 1 and check(rows[-1]):
            return rows[-1]
        time.sleep(0.5)
    raise AssertionError("watcher drill timed out")
wait(lambda i: i["Brief"] and present(i["FixProposed"]))
subprocess.run(os.environ["SCRY_DRILL_KILL_CMD"], shell=True, check=True, timeout=30)
result = wait(lambda i: present(i["Recovered"]) and present(i["Resolved"]) and i["Count"] > 1)
assert urllib.request.urlopen(url + "/healthz", timeout=5).status == 200
print("PASS: one incident, triage, fix handoff, killed-process recovery and quiet close")
print("Incident:", result["ID"], "recovery seconds:", result["RecoverySeconds"])
